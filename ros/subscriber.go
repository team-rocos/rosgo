package ros

import (
	"bytes"
	goContext "context"
	"fmt"
	"reflect"
	"sync"
	"time"

	modular "github.com/edwinhayes/logrus-modular"
	"github.com/pkg/errors"
)

type messageEvent struct {
	bytes []byte
	event MessageEvent
}

type subscriptionChannels struct {
	enableMessages chan bool
}

// SubscriberRos interface provides methods to decouple ROS API calls from the subscriber itself.
type SubscriberRos interface {
	RequestTopicURI(pub string) (string, error)
	Unregister() error
}

// SubscriberRosAPI implements SubscriberRos using callRosAPI rpc calls.
type SubscriberRosAPI struct {
	topic      string
	nodeID     string
	nodeAPIURI string
	masterURI  string
}

// RequestTopicURI requests the URI of a given topic from a publisher.
func (a *SubscriberRosAPI) RequestTopicURI(pub string) (string, error) {
	protocols := []interface{}{[]interface{}{"TCPROS"}}
	result, err := callRosAPI(pub, "requestTopic", a.nodeID, a.topic, protocols)

	if err != nil {
		return "", err
	}

	protocolParams := result.([]interface{})

	if name := protocolParams[0].(string); name != "TCPROS" {
		return "", errors.New("rosgo does not support protocol: " + name)
	}

	addr := protocolParams[1].(string)
	port := protocolParams[2].(int32)
	uri := fmt.Sprintf("%s:%d", addr, port)
	return uri, nil
}

// Unregister removes a subscriber from a topic.
func (a *SubscriberRosAPI) Unregister() error {
	_, err := callRosAPI(a.masterURI, "unregisterSubscriber", a.nodeID, a.topic, a.nodeAPIURI)
	return err
}

var _ SubscriberRos = &SubscriberRosAPI{}

// startPublosherSubscription defines a function interface for starting a subscription in run.
type startPublisherSubscription func(ctx goContext.Context, pubURI string, log *modular.ModuleLogger)

// The subscriber object runs in own goroutine (start).
type defaultSubscriber struct {
	topic            string
	msgType          MessageType
	pubList          []string
	pubListChan      chan []string
	msgChan          chan messageEvent
	callbacks        []interface{}
	addCallbackChan  chan interface{}
	shutdownChan     chan struct{}
	cancel           map[string]goContext.CancelFunc
	uri2pub          map[string]string
	disconnectedChan chan string
}

func newDefaultSubscriber(topic string, msgType MessageType, callback interface{}) *defaultSubscriber {
	sub := new(defaultSubscriber)
	sub.topic = topic
	sub.msgType = msgType
	sub.msgChan = make(chan messageEvent)
	sub.pubListChan = make(chan []string, 10)
	sub.addCallbackChan = make(chan interface{}, 10)
	sub.shutdownChan = make(chan struct{})
	sub.disconnectedChan = make(chan string, 10)
	sub.uri2pub = make(map[string]string)
	sub.cancel = make(map[string]goContext.CancelFunc) // TODO: move this out of here... it belongs in the run routine
	sub.callbacks = []interface{}{callback}
	return sub
}

func (sub *defaultSubscriber) start(wg *sync.WaitGroup, nodeID string, nodeAPIURI string, masterURI string, jobChan chan func(), enableChan chan bool, log *modular.ModuleLogger) {
	ctx, cancel := goContext.WithCancel(goContext.Background())
	defer cancel()
	logger := *log
	logger.Debugf("Subscriber goroutine for %s started.", sub.topic)

	wg.Add(1)
	defer wg.Done()
	defer func() {
		logger.Debug(sub.topic, " : defaultSubscriber.start exit")
	}()

	// Construct the SubscriberRosApi.
	rosAPI := &SubscriberRosAPI{
		topic:      sub.topic,
		nodeID:     nodeID,
		masterURI:  masterURI,
		nodeAPIURI: nodeAPIURI,
	}

	// Decouples a bunch of implementation details from the actual run logic.
	startSubscription := func(ctx goContext.Context, pubURI string, log *modular.ModuleLogger) {
		startRemotePublisherConn(ctx, &TCPRosNetDialer{}, pubURI, sub.topic, sub.msgType, nodeID, sub.msgChan, sub.disconnectedChan, log)
	}

	// Setup is complete, run the subscriber.
	sub.run(ctx, jobChan, enableChan, rosAPI, startSubscription, log)
}

func (sub *defaultSubscriber) run(ctx goContext.Context, jobChan chan func(), enableChan chan bool, rosAPI SubscriberRos, startSubscription startPublisherSubscription, log *modular.ModuleLogger) {
	logger := *log
	enabled := true

	for {
		select {
		case list := <-sub.pubListChan:
			logger.Debug(sub.topic, " : Receive pubListChan")
			deadPubs := setDifference(sub.pubList, list)
			newPubs := setDifference(list, sub.pubList)
			// TODO:
			// sub.pubList = setDifference(sub.pubList, deadPubs)
			sub.pubList = list
			for _, pub := range deadPubs {
				if subCancel, ok := sub.cancel[pub]; ok {
					subCancel()
					delete(sub.cancel, pub)
				}
			}

			// TODO:
			// make into a go routine, give it a channel requestTopicResult chan (pub string, uri string, err error)
			for _, pub := range newPubs {
				uri, err := rosAPI.RequestTopicURI(pub)
				if err != nil {
					logger.Error("uri request failed, topic : ", sub.topic, ", error : ", err)
					continue
				}

				// TODO:
				// Everything past here doesn't need to be in the go routine, it should be handled on receiving from the requestTopicResult channel
				sub.uri2pub[uri] = pub
				subCtx, subCancel := goContext.WithCancel(ctx)
				defer subCancel()
				// TODO:
				// sub.pubList = append(sub.pubList, pub)
				sub.cancel[pub] = subCancel
				startSubscription(subCtx, uri, log)
			}

		case pubURI := <-sub.disconnectedChan:
			logger.Debugf("Connection to %s was disconnected.", pubURI)
			if pub, ok := sub.uri2pub[pubURI]; ok {
				if subCancel, ok := sub.cancel[pub]; ok {
					subCancel()
					delete(sub.cancel, pub)
				}
				delete(sub.uri2pub, pubURI)
			}

		case callback := <-sub.addCallbackChan:
			logger.Debug(sub.topic, " : Receive addCallbackChan")
			sub.callbacks = append(sub.callbacks, callback)

		case msgEvent := <-sub.msgChan:
			if enabled == false {
				continue
			}
			// Pop received message then bind callbacks and enqueue to the job channel.
			logger.Debug(sub.topic, " : Receive msgChan")

			callbacks := make([]interface{}, len(sub.callbacks))
			copy(callbacks, sub.callbacks)
			// TODO: Move this to the same pattern used in subscriber, should be:
			// latestJob := func() { .... }
			// activeJobChan = jobChan
			//
			// then in the main for-select loop, we have:
			// case activeJobChan <- latestJob:
			//   activeJobChan = nil
			//   latestJob = func(){}
			select {
			case jobChan <- func() {
				m := sub.msgType.NewMessage()
				reader := bytes.NewReader(msgEvent.bytes)
				if err := m.Deserialize(reader); err != nil {
					logger.Error(sub.topic, " : ", err)
				}
				// TODO: Investigate this
				args := []reflect.Value{reflect.ValueOf(m), reflect.ValueOf(msgEvent.event)}
				for _, callback := range callbacks {
					fun := reflect.ValueOf(callback)
					numArgsNeeded := fun.Type().NumIn()
					if numArgsNeeded <= 2 {
						fun.Call(args[0:numArgsNeeded])
					}
				}
			}:
				logger.Debug(sub.topic, " : Callback job enqueued.")
			// TODO: Eliminate this nasty bastard
			case <-time.After(time.Duration(3) * time.Second):
				logger.Debug(sub.topic, " : Callback job timed out.")
			}
			logger.Debug("Callback job enqueued.")

		case <-sub.shutdownChan:
			// Shutdown subscription goroutine; keeps shutdowns snappy.
			go func() {
				logger.Debug(sub.topic, " : receive shutdownChan")
				if err := rosAPI.Unregister(); err != nil {
					logger.Warn(sub.topic, " : unregister error: ", err)
				}
			}()
			sub.shutdownChan <- struct{}{}
			return

		case enabled = <-enableChan:
		}
	}
}

// TODO:
// Will simplify testing a lot if we are able to mock this out... something like:
// `startPublisherSubscription`

// startRemotePublisherConn creates a subscription to a remote publisher and runs it.
func startRemotePublisherConn(ctx goContext.Context, dialer TCPRosDialer,
	pubURI string, topic string, msgType MessageType, nodeID string,
	msgChan chan messageEvent,
	disconnectedChan chan string,
	log *modular.ModuleLogger) {
	sub := newDefaultSubscription(pubURI, topic, msgType, nodeID, msgChan, disconnectedChan)
	sub.dialer = dialer
	sub.startWithContext(ctx, log)
}

// TODO: Put tests on this
func setDifference(lhs []string, rhs []string) []string {
	left := map[string]bool{}
	for _, item := range lhs {
		left[item] = true
	}
	right := map[string]bool{}
	for _, item := range rhs {
		right[item] = true
	}
	for k := range right {
		delete(left, k)
	}
	var result []string
	for k := range left {
		result = append(result, k)
	}
	return result
}

func (sub *defaultSubscriber) Shutdown() {
	sub.shutdownChan <- struct{}{}
	<-sub.shutdownChan
}

func (sub *defaultSubscriber) GetNumPublishers() int {
	return len(sub.pubList)
}
