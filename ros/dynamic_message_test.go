package ros

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	gengo "github.com/team-rocos/rosgo/libgengo"
)

func TestDynamicMessage_TypeGetters(t *testing.T) {
	fields := []gengo.Field{
		*gengo.NewField("Testing", "float32", "x", false, 0),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	if testMessageType.Name() != "TestMessage" {
		t.Fatalf("DynamicMessageType has unexpected Name %s", testMessageType.Name())
	}

	if testMessageType.MD5Sum() != "1337beeffeed1337" {
		t.Fatalf("DynamicMessageType has unexpected MD5Sum %s", testMessageType.MD5Sum())
	}
}

func TestDynamicMessage_Deserialize_Simple(t *testing.T) {
	fields := []gengo.Field{
		*gengo.NewField("Testing", "float32", "x", false, 0),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	// Using IEEE754 https://www.h-schmidt.net/FloatConverter/IEEE754.html
	// 1234.5678 = 0x449a522b
	// Then convert to little-endian.
	expected := float32(1234.5678)
	byteReader := bytes.NewReader([]byte{0x2b, 0x52, 0x9a, 0x44})

	testMessage := testMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err != nil {
		t.Fatalf("deserialize failed %s", err)
	}

	xWrapped, ok := testMessage.data["x"]
	if ok == false {
		t.Fatalf("failed to deserialize x, got %s", testMessage.data)
	}

	x, ok := xWrapped.(JsonFloat32)
	if ok == false {
		t.Fatalf("x is not a float32, got %s", testMessage.data)
	}

	if !Float32Near(expected, x.F, 1e-4) {
		t.Fatalf("x (%f) is not near %f", x, expected)
	}
}

func TestDynamicMessage_DynamicType_Load(t *testing.T) {
	poseMessageType, err := NewDynamicMessageType("geometry_msgs/Pose")

	if err != nil {
		t.Skip("test skipped because ROS environment not set up")
		return
	}

	if len(poseMessageType.spec.Fields) != 2 {
		t.Fatalf("expected 2 pose fields")
	}

	// Ensure that we have embedded additional DynamicMessageTypes for Point and Quaternion.
	if len(poseMessageType.nested) != 5 {
		t.Fatalf("expected 5 nested message types, got %v", poseMessageType.nested)
	}

	if pointType, ok := poseMessageType.nested["position"]; ok {
		if pointType.spec.FullName != "geometry_msgs/Point" {
			t.Fatalf("expected nested Point, got %s", pointType.spec.FullName)
		}
		if len(pointType.spec.Fields) != 3 {
			t.Fatalf("expected 3 fields for nested Point type")
		}
	} else {
		t.Fatalf("expected point type under nested[\"position\"]")
	}

	if quatType, ok := poseMessageType.nested["orientation"]; ok {
		if quatType.spec.FullName != "geometry_msgs/Quaternion" {
			t.Fatalf("expected nested Quaternion, got %s", quatType.spec.FullName)
		}
		if len(quatType.spec.Fields) != 4 {
			t.Fatalf("expected 4 fields for nested Quaternion type")
		}
	} else {
		t.Fatalf("expected quaternion type under nested[\"orientation\"]")
	}

	// Pose has 7 float64 values, 7 x 8 bytes = 56 bytes.
	slice := make([]byte, 56)
	byteReader := bytes.NewReader(slice)

	testMessage := poseMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err != nil {
		t.Fatalf("deserialize pose failed")
	}

	pos, ok := testMessage.Data()["position"]
	if !ok {
		t.Fatalf("failed to get position from pose message")
	}

	if _, ok = pos.(*DynamicMessage).Data()["x"]; !ok {
		t.Fatalf("failed to get position.x from pose message")
	}
}

func TestDynamicMessage_TypeWithRecursion(t *testing.T) {
	// We don't care about Pose in this step, but we want to load libgengo's context.
	_, err := NewDynamicMessageType("geometry_msgs/Pose")

	if err != nil {
		t.Skip("test skipped because ROS environment not set up")
		return
	}
	fields := []gengo.Field{
		*gengo.NewField("test", "recursiveMessage", "x", true, -1),
	}
	msgSpec := generateTestSpec(fields)
	context.RegisterMsg("recursiveMessage", msgSpec)

	_, err = NewDynamicMessageType("recursiveMessage") // If this isn't handled correctly, we get stack overflow.

	if err == nil {
		t.Fatal("recursive message defintion did not result in an error")
	}
}

func TestDynamicMessage_TypeWithBuriedRecursion(t *testing.T) {
	// We don't care about Pose in this step, but we want to load libgengo's context.
	_, err := NewDynamicMessageType("geometry_msgs/Pose")

	if err != nil {
		t.Skip("test skipped because ROS environment not set up")
		return
	}
	// Recursive pattern is x->y->z.
	fields := []gengo.Field{
		*gengo.NewField("test", "yMessage", "y", true, -1),
	}
	msgSpec := generateTestSpec(fields)
	context.RegisterMsg("xMessage", msgSpec)

	fields = []gengo.Field{
		*gengo.NewField("test", "zMessage", "z", true, -1),
	}
	msgSpec = generateTestSpec(fields)
	context.RegisterMsg("yMessage", msgSpec)

	fields = []gengo.Field{
		*gengo.NewField("test", "xMessage", "x", true, -1),
	}
	msgSpec = generateTestSpec(fields)
	context.RegisterMsg("zMessage", msgSpec)

	_, err = NewDynamicMessageType("xMessage") // If this isn't handled correctly, we get stack overflow.

	if err == nil {
		t.Fatal("recursive message defintion did not result in an error")
	}
}

func TestDynamicMessage_Deserialize_Unknown(t *testing.T) {
	fields := []gengo.Field{
		*gengo.NewField("Testing", "Unknown", "x", false, 0),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	// The unknown type isn't real, so just give it some junk bytes.
	byteReader := bytes.NewReader([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

	testMessage := testMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err == nil {
		t.Fatalf("deserialized unknown type, expected failure")
	}
}

func TestDynamicMessage_Deserialize_SingularMedley(t *testing.T) {
	fields := []gengo.Field{
		*gengo.NewField("Testing", "uint8", "u8", false, 0),
		*gengo.NewField("Testing", "uint16", "u16", false, 0),
		*gengo.NewField("Testing", "uint32", "u32", false, 0),
		*gengo.NewField("Testing", "uint64", "u64", false, 0),
		*gengo.NewField("Testing", "int8", "i8", false, 0),
		*gengo.NewField("Testing", "int16", "i16", false, 0),
		*gengo.NewField("Testing", "int32", "i32", false, 0),
		*gengo.NewField("Testing", "int64", "i64", false, 0),
		*gengo.NewField("Testing", "bool", "b", false, 0),
		*gengo.NewField("Testing", "float32", "f32", false, 0),
		*gengo.NewField("Testing", "float64", "f64", false, 0),
		*gengo.NewField("Testing", "string", "s", false, 0),
		*gengo.NewField("Testing", "time", "t", false, 0),
		*gengo.NewField("Testing", "duration", "d", false, 0),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	var expected = map[string]interface{}{
		"u8":  uint8(0x12),
		"u16": uint16(0x3456),
		"u32": uint32(0x789abcde),
		"u64": uint64(0x123456789abcdef0),
		"i8":  int8(-2),
		"i16": int16(-2),
		"i32": int32(-2),
		"i64": int64(-2),
		"b":   true,
		"f32": JsonFloat32{1234.5678},  // 1234.5678 = 0x449a522b
		"f64": JsonFloat64{-9876.5432}, // 0xC0C3 4A45 8793 DD98
		"s":   "Rocos",
		"t":   NewTime(0xfeedf00d, 0x1337beef),
		"d":   NewDuration(0x50607080, 0x10203040),
	}

	byteReader := bytes.NewReader([]byte{
		0x12,       // u8
		0x56, 0x34, // u16
		0xde, 0xbc, 0x9a, 0x78, // u32
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u64
		0xfe,       // i8
		0xfe, 0xff, // i16
		0xfe, 0xff, 0xff, 0xff, // i32
		0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // i64
		0x01,                   // bool
		0x2b, 0x52, 0x9a, 0x44, // f32
		0x98, 0xdd, 0x93, 0x87, 0x45, 0x4a, 0xc3, 0xc0, // f64
		0x05, 0x00, 0x00, 0x00, 'R', 'o', 'c', 'o', 's', // s
		0x0d, 0xf0, 0xed, 0xfe, 0xef, 0xbe, 0x37, 0x13, // t
		0x80, 0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, // d
	})

	testMessage := testMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err != nil {
		t.Fatalf("deserialize failed %s", err)
	}

	// Check that our resulting data matches our expected result.
	for key := range expected {
		value, ok := testMessage.data[key]
		if !ok {
			t.Fatalf("failed to deserialize %s, got %s", key, testMessage.data)
		}

		var expectedValue interface{} = expected[key]
		if expectedValue != value {
			t.Fatalf("%s: expected %d(0x%x) != result %d(0x%x)", key, expectedValue, expectedValue, value, value)
		}
	}
}

func TestDynamicMessage_Deserialize_FixedArrayMedley(t *testing.T) {
	fields := []gengo.Field{
		*gengo.NewField("Testing", "uint8", "u8", true, 8),
		*gengo.NewField("Testing", "uint16", "u16", true, 4),
		*gengo.NewField("Testing", "uint32", "u32", true, 2),
		*gengo.NewField("Testing", "uint64", "u64", true, 1),
		*gengo.NewField("Testing", "int8", "i8", true, 8),
		*gengo.NewField("Testing", "int16", "i16", true, 4),
		*gengo.NewField("Testing", "int32", "i32", true, 2),
		*gengo.NewField("Testing", "int64", "i64", true, 1),
		*gengo.NewField("Testing", "bool", "b", true, 8),
		*gengo.NewField("Testing", "float32", "f32", true, 2),
		*gengo.NewField("Testing", "float64", "f64", true, 1),
		*gengo.NewField("Testing", "string", "s", true, 3),
		*gengo.NewField("Testing", "time", "t", true, 2),
		*gengo.NewField("Testing", "duration", "d", true, 2),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	var expected = map[string]interface{}{
		"u8":  []uint8{0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12},
		"u16": []uint16{0xdef0, 0x9abc, 0x5678, 0x1234},
		"u32": []uint32{0x9abcdef0, 0x12345678},
		"u64": []uint64{0x123456789abcdef0},
		"i8":  []int8{-2, -1, 0, 1, 2, 3, 4, 5},
		"i16": []int16{-2, -1, 0, 1},
		"i32": []int32{-2, 1},
		"i64": []int64{-2},
		"b":   []bool{true, true, false, false, true, false, true, false},
		"f32": []JsonFloat32{{1234.5678}, {1234.5678}}, // 1234.5678 = 0x449a522b
		"f64": []JsonFloat64{{-9876.5432}},             // -9876.5432 = 0xC0C3 4A45 8793 DD98
		"s":   []string{"Rocos", "soroc", "croos"},
		"t":   []Time{NewTime(0xfeedf00d, 0x1337beef), NewTime(0x1337beef, 0x1337f00d)},
		"d":   []Duration{NewDuration(0x40302010, 0x00706050), NewDuration(0x50607080, 0x10203040)},
	}

	byteReader := bytes.NewReader([]byte{
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u8
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u16
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u32
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u64
		0xfe, 0xff, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, // i8
		0xfe, 0xff, 0xff, 0xff, 0x00, 0x00, 0x01, 0x00, // i16
		0xfe, 0xff, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00, // i32
		0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // i64
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, // b
		0x2b, 0x52, 0x9a, 0x44, 0x2b, 0x52, 0x9a, 0x44, // f32
		0x98, 0xdd, 0x93, 0x87, 0x45, 0x4a, 0xc3, 0xc0, // f64
		0x05, 0x00, 0x00, 0x00, 'R', 'o', 'c', 'o', 's', // s[0]
		0x05, 0x00, 0x00, 0x00, 's', 'o', 'r', 'o', 'c', // s[1]
		0x05, 0x00, 0x00, 0x00, 'c', 'r', 'o', 'o', 's', // s[2]
		0x0d, 0xf0, 0xed, 0xfe, 0xef, 0xbe, 0x37, 0x13, // t[0]
		0xef, 0xbe, 0x37, 0x13, 0x0d, 0xf0, 0x37, 0x13, // t[1]
		0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x00, // d[0]
		0x80, 0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, // d[1]
	})

	testMessage := testMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err != nil {
		t.Fatalf("deserialize failed %s", err)
	}

	// Check that our resulting data matches our expected result.
	for key := range expected {
		value, ok := testMessage.data[key]
		if !ok {
			t.Fatalf("failed to deserialize %s, got %s", key, testMessage.data)
		}

		expectedValue := expected[key]
		if fmt.Sprint(expectedValue) != fmt.Sprint(value) {
			t.Fatalf("%s: expected %d(0x%x) != result %d(0x%x)", key, expectedValue, expectedValue, value, value)
		}
	}
}

func TestDynamicMessage_Deserialize_DynamicArrayMedley(t *testing.T) {
	// Dynamic array type used for testing across all ROS primitives. Note: negative array sizes => dynamic arrays.
	fields := []gengo.Field{
		*gengo.NewField("Testing", "uint8", "u8", true, -1),
		*gengo.NewField("Testing", "uint16", "u16", true, -1),
		*gengo.NewField("Testing", "uint32", "u32", true, -1),
		*gengo.NewField("Testing", "uint64", "u64", true, -1),
		*gengo.NewField("Testing", "int8", "i8", true, -1),
		*gengo.NewField("Testing", "int16", "i16", true, -1),
		*gengo.NewField("Testing", "int32", "i32", true, -1),
		*gengo.NewField("Testing", "int64", "i64", true, -1),
		*gengo.NewField("Testing", "bool", "b", true, -1),
		*gengo.NewField("Testing", "float32", "f32", true, -1),
		*gengo.NewField("Testing", "float64", "f64", true, -1),
		*gengo.NewField("Testing", "string", "s", true, -1),
		*gengo.NewField("Testing", "time", "t", true, -1),
		*gengo.NewField("Testing", "duration", "d", true, -1),
	}
	testMessageType := DynamicMessageType{
		generateTestSpec(fields),
		make(map[string]*DynamicMessageType),
	}

	var expected = map[string]interface{}{
		"u8":  []uint8{0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12},
		"u16": []uint16{0xdef0, 0x9abc, 0x5678, 0x1234},
		"u32": []uint32{0x9abcdef0, 0x12345678},
		"u64": []uint64{0x123456789abcdef0},
		"i8":  []int8{-2, -1, 0, 1, 2, 3, 4, 5},
		"i16": []int16{-2, -1, 0, 1},
		"i32": []int32{-2, 1},
		"i64": []int64{-2},
		"b":   []bool{true, true, false, false, true, false, true, false},
		"f32": []JsonFloat32{{1234.5678}, {1234.5678}}, // 1234.5678 = 0x449A 522B
		"f64": []JsonFloat64{{-9876.5432}},             // -9876.5432 = 0xC0C3 4A45 8793 DD98
		"s":   []string{"Rocos", "soroc", "croos"},
		"t":   []Time{NewTime(0xfeedf00d, 0x1337beef), NewTime(0x1337beef, 0x1337f00d)},
		"d":   []Duration{NewDuration(0x40302010, 0x00706050), NewDuration(0x50607080, 0x10203040)},
	}

	byteReader := bytes.NewReader([]byte{
		0x08, 0x00, 0x00, 0x00, // Dynamic array size.
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u8
		0x04, 0x00, 0x00, 0x00, // Dynamic array size.
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u16
		0x02, 0x00, 0x00, 0x00, // Dynamic array size.
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u32
		0x01, 0x00, 0x00, 0x00, // Dynamic array size.
		0xf0, 0xde, 0xbc, 0x9a, 0x78, 0x56, 0x34, 0x12, // u64
		0x08, 0x00, 0x00, 0x00, // Dynamic array size.
		0xfe, 0xff, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, // i8
		0x04, 0x00, 0x00, 0x00, // Dynamic array size.
		0xfe, 0xff, 0xff, 0xff, 0x00, 0x00, 0x01, 0x00, // i16
		0x02, 0x00, 0x00, 0x00, // Dynamic array size.
		0xfe, 0xff, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00, // i32
		0x01, 0x00, 0x00, 0x00, // Dynamic array size.
		0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, // i64
		0x08, 0x00, 0x00, 0x00, // Dynamic array size.
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, // b
		0x02, 0x00, 0x00, 0x00, // Dynamic array size.
		0x2b, 0x52, 0x9a, 0x44, 0x2b, 0x52, 0x9a, 0x44, // f32
		0x01, 0x00, 0x00, 0x00, // Dynamic array size.
		0x98, 0xdd, 0x93, 0x87, 0x45, 0x4a, 0xc3, 0xc0, // f64
		0x03, 0x00, 0x00, 0x00, // Dynamic array size.
		0x05, 0x00, 0x00, 0x00, 'R', 'o', 'c', 'o', 's', // s[0]
		0x05, 0x00, 0x00, 0x00, 's', 'o', 'r', 'o', 'c', // s[1]
		0x05, 0x00, 0x00, 0x00, 'c', 'r', 'o', 'o', 's', // s[2]
		0x02, 0x00, 0x00, 0x00, // Dynamic array size.
		0x0d, 0xf0, 0xed, 0xfe, 0xef, 0xbe, 0x37, 0x13, // t[0]
		0xef, 0xbe, 0x37, 0x13, 0x0d, 0xf0, 0x37, 0x13, // t[1]
		0x02, 0x00, 0x00, 0x00, // Dynamic array size.
		0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x00, // d[0]
		0x80, 0x70, 0x60, 0x50, 0x40, 0x30, 0x20, 0x10, // d[1]
	})

	testMessage := testMessageType.NewDynamicMessage()

	if err := testMessage.Deserialize(byteReader); err != nil {
		t.Fatalf("deserialize failed %s", err)
	}

	// Test our resulting data
	for key := range expected {
		value, ok := testMessage.data[key]
		if !ok {
			t.Fatalf("failed to deserialize %s, got %s", key, testMessage.data)
		}

		expectedValue := expected[key]
		if fmt.Sprint(expectedValue) != fmt.Sprint(value) {
			t.Fatalf("%s: expected %d(0x%x) != result %d(0x%x)", key, expectedValue, expectedValue, value, value)
		}
	}
}

// Testing helpers

// Float32Near helper to check that two float32 are within a tolerance.
func Float32Near(expected float32, actual float32, tol float32) bool {
	return math.Abs(float64(expected-actual)) < float64(tol)
}

// generateTestSpec creates a message spec for a ficticious message type.
func generateTestSpec(fields []gengo.Field) *gengo.MsgSpec {
	msgSpec := &gengo.MsgSpec{}
	msgSpec.FullName = "TestMessage"
	msgSpec.Package = "Testing"
	msgSpec.MD5Sum = "1337beeffeed1337"
	msgSpec.ShortName = "Test"
	msgSpec.Fields = fields
	return msgSpec
}
