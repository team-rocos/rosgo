package libgengo

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const (
	HeaderType     = "Header"
	TimeType       = "time"
	DurationType   = "duration"
	HeaderFullName = "std_msgs/Header"
	TimeMsg        = "uint32 secs\nuint32 nsecs"
	DurationMsg    = "uint32 secs\nuint32 nsecs"
)

// BuiltInType enumeration represents a standard ros type. See http://wiki.ros.org/msg for specification.
type BuiltInType int

// Enumeration of all ros builtin types. Invalid represents a non-built in type.
const (
	Invalid BuiltInType = iota
	Bool
	Int8
	Int16
	Int32
	Int64
	Uint8
	Uint16
	Uint32
	Uint64
	Float32
	Float64
	String
	Time
	Duration
)

var PrimitiveTypes = []string{
	"int8",
	"uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "float32", "float64",
	"string",
	"bool",
	// deprecated:
	"char", "byte",
}

var BuiltinTypes = append([]string{TimeType, DurationType}, PrimitiveTypes...)

var ResourceNameLegalCharsPattern = regexp.MustCompile(`^[A-Za-z][\w_\/]*$`)

var BaseResourceNameLegalCharsPattern = regexp.MustCompile(`"[A-Za-z][\w_]*$`)

func isValidConsantType(t string) bool {
	for _, e := range PrimitiveTypes {
		if e == t {
			return true
		}
	}
	return false
}

func isValidMsgFieldName(name string) bool {
	return isLegalResourceBaseName(name)
}

func isLegalResourceBaseName(name string) bool {
	if strings.Contains(name, "//") {
		return false
	}
	return ResourceNameLegalCharsPattern.MatchString(name)
}

func isLegalResourceName(name string) bool {
	return BaseResourceNameLegalCharsPattern.MatchString(name)
}

func isPrimitiveType(name string) bool {
	for _, t := range PrimitiveTypes {
		if t == name {
			return true
		}
	}
	return false
}

func isBuiltinType(name string) bool {
	for _, t := range BuiltinTypes {
		if t == name {
			return true
		}
	}
	return false
}

func baseMsgType(t string) string {
	index := strings.Index(t, "[")
	if index < 0 {
		return t
	} else {
		return t[:index]
	}
}

func splitType(t string) (string, string) {
	components := strings.Split(t, "/")
	if len(components) == 1 {
		return "", t
	} else {
		return components[0], components[1]
	}
}

func parseType(msgType string) (pkg string, baseType string, isArray bool, arrayLen int, err error) {
	index := strings.Index(msgType, "[")
	if index < 0 {
		pkg, name := splitType(msgType)
		return pkg, name, false, 0, nil
	} else {
		if msgType[len(msgType)-1] == ']' {
			base := msgType[:index]
			rest := msgType[index:]
			pkg, name := splitType(base)
			if rest == "[]" {
				return pkg, name, true, -1, nil
			} else {
				value64, err := strconv.ParseInt(rest[1:len(rest)-1], 10, 32)
				if err != nil {
					return pkg, name, false, 0, err
				}
				value := int(value64)
				return pkg, name, true, value, nil
			}
		} else {
			return "", msgType, false, 0, fmt.Errorf("missing ']'")
		}
	}
}

func isValidMsgType(t string) bool {
	if t != strings.TrimSpace(t) {
		return false
	}
	base := baseMsgType(t)
	if !isLegalResourceBaseName(base) {
		return false
	}

	x := t[len(base):]
	state := 0
	for _, c := range x {
		if state == 0 {
			if c != '[' {
				return false
			}
			state = 1
		} else if state == 1 {
			if c == ']' {
				state = 0
			} else if !unicode.IsDigit(c) {
				return false
			}
		}
	}
	return state == 0
}

func isValidConstantType(t string) bool {
	for _, pt := range PrimitiveTypes {
		if t == pt {
			return true
		}
	}
	return false
}

func isHeaderType(name string) bool {
	patterns := map[string]bool{
		HeaderType:      true,
		HeaderFullName:  true,
		"roslib/Header": true,
	}
	return patterns[name]
}

type Constant struct {
	Type      string
	Name      string
	Value     interface{}
	ValueText string
	GoName    string
}

func ToGoType(pkg string, typeName string) string {
	var goType string
	switch typeName {
	case "int8":
		goType = "int8"
	case "uint8":
		goType = "uint8"
	case "int16":
		goType = "int16"
	case "uint16":
		goType = "uint16"
	case "int32":
		goType = "int32"
	case "uint32":
		goType = "uint32"
	case "int64":
		goType = "int64"
	case "uint64":
		goType = "uint64"
	case "float32":
		goType = "float32"
	case "float64":
		goType = "float64"
	case "string":
		goType = "string"
	case "bool":
		goType = "bool"
	case "char":
		goType = "uint8"
	case "byte":
		goType = "uint8"
	case "time":
		goType = "ros.Time"
	case "duration":
		goType = "ros.Duration"
	default:
		goType = pkg + "." + typeName
	}
	return goType
}

func ToBuiltInType(typeName string) BuiltInType {
	var builtInType BuiltInType
	switch typeName {
	case "int8":
		builtInType = Int8
	case "uint8", "char", "byte":
		builtInType = Uint8
	case "int16":
		builtInType = Int16
	case "uint16":
		builtInType = Uint16
	case "int32":
		builtInType = Int32
	case "uint32":
		builtInType = Uint32
	case "int64":
		builtInType = Int64
	case "uint64":
		builtInType = Uint64
	case "float32":
		builtInType = Float32
	case "float64":
		builtInType = Float64
	case "string":
		builtInType = String
	case "bool":
		builtInType = Bool
	case "time":
		builtInType = Time
	case "duration":
		builtInType = Duration
	default:
		builtInType = Invalid
	}
	return builtInType
}

func ToGoName(name string, constant bool) string {
	if constant {
		return strings.ToUpper(name)
	}

	var buffer []string
	words := strings.Split(name, "_")
	for _, word := range words {
		if len(word) > 0 {
			head := strings.ToUpper(word[:1])
			tail := ""
			if len(word) > 1 {
				tail = word[1:]
			}
			buffer = append(append(buffer, head), tail)
		}
	}
	return strings.Join(buffer, "")
}

func GetZeroValue(pkg string, typeName string) string {
	var zeroValue string
	switch typeName {
	case "int8":
		zeroValue = "0"
	case "uint8":
		zeroValue = "0"
	case "int16":
		zeroValue = "0"
	case "uint16":
		zeroValue = "0"
	case "int32":
		zeroValue = "0"
	case "uint32":
		zeroValue = "0"
	case "int64":
		zeroValue = "0"
	case "uint64":
		zeroValue = "0"
	case "float32":
		zeroValue = "0.0"
	case "float64":
		zeroValue = "0.0"
	case "string":
		zeroValue = "\"\""
	case "bool":
		zeroValue = "false"
	case "char":
		zeroValue = "0"
	case "byte":
		zeroValue = "0"
	case "time":
		zeroValue = "ros.Time{}"
	case "duration":
		zeroValue = "ros.Duration{}"
	default:
		zeroValue = pkg + "." + typeName + "{}"
	}
	return zeroValue
}

func NewConstant(fieldType string, name string, value interface{}, valueText string) *Constant {
	goName := ToGoName(name, true)
	return &Constant{fieldType, name, value, valueText, goName}
}

func (c *Constant) String() string {
	return fmt.Sprintf("%s %s = %v", c.Type, c.Name, c.Value)
}

type Field struct {
	Package     string
	Type        string
	Name        string
	IsBuiltin   bool
	BuiltInType BuiltInType
	IsArray     bool
	ArrayLen    int
	GoName      string
	GoType      string
	ZeroValue   string
}

func NewField(pkg string, fieldType string, name string, isArray bool, arrayLen int) *Field {
	builtInType := ToBuiltInType(fieldType)
	goType := ToGoType(pkg, fieldType)
	goName := ToGoName(name, false)
	zeroValue := GetZeroValue(pkg, fieldType)
	isBuiltin := builtInType != Invalid
	return &Field{pkg, fieldType, name, isBuiltin, builtInType, isArray, arrayLen, goName, goType, zeroValue}
}

func (f *Field) String() string {
	if f.IsArray && f.ArrayLen > -1 {
		return fmt.Sprintf("%s[%d] %s", f.Type, f.ArrayLen, f.Name)
	} else if f.IsArray {
		return fmt.Sprintf("%s[] %s", f.Type, f.Name)
	} else {
		return fmt.Sprintf("%s %s", f.Type, f.Name)
	}
}

type MsgSpec struct {
	Fields    []Field
	Constants []Constant
	Text      string
	MD5Sum    string
	FullName  string
	ShortName string
	Package   string
}

type SrvSpec struct {
	Package   string
	ShortName string
	FullName  string
	Text      string
	MD5Sum    string
	Request   *MsgSpec
	Response  *MsgSpec
}

type ActionSpec struct {
	Package        string
	ShortName      string
	FullName       string
	Text           string
	MD5Sum         string
	Goal           *MsgSpec
	Feedback       *MsgSpec
	Result         *MsgSpec
	ActionGoal     *MsgSpec
	ActionFeedback *MsgSpec
	ActionResult   *MsgSpec
}

type OptionMsgSpec func(*MsgSpec) error

func OptionPackageName(name string) func(*MsgSpec) error {
	return func(spec *MsgSpec) error {
		spec.Package = name
		return nil
	}
}

func OptionShortName(name string) func(*MsgSpec) error {
	return func(spec *MsgSpec) error {
		spec.ShortName = name
		return nil
	}
}

func NewMsgSpec(fields []Field, constants []Constant, text string, fullName string, options ...OptionMsgSpec) (*MsgSpec, error) {
	spec := &MsgSpec{}

	spec.Fields = fields
	spec.Constants = constants
	spec.Text = text
	spec.FullName = fullName

	for _, opt := range options {
		err := opt(spec)
		if err != nil {
			return nil, err
		}
	}
	return spec, nil
}

// Implements Stringer interface
func (s *MsgSpec) String() string {
	lines := []string{}

	lines = append(lines, fmt.Sprintf("msg %s {", s.FullName))

	for _, c := range s.Constants {
		lines = append(lines, fmt.Sprintf("\t%s", c.String()))
	}
	lines = append(lines, "")
	for _, f := range s.Fields {
		lines = append(lines, fmt.Sprintf("\t%s", f.String()))
	}

	lines = append(lines, fmt.Sprintf("}"))
	return strings.Join(lines, "\n")
}
