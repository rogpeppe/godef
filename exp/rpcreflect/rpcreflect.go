package rpcreflect
import (
	"fmt"
	"encoding/json"
	"reflect"
	"strings"
)

// simple types:
// string -> "string"
// int8, int64 etc -> "number"
// interface{} -> "any"
// 
// composite types:
// slice, array -> [elemType]
// struct -> {"Field1": field1Type, "Field2": field2Type, etc}
// map -> {"_map": elemType}
// pointer -> {"_nullable": elemType}
// 
// types with custom MarshalJSON methods:
// "foo.Bar" (type name)
// 
// what about self-referential data structures?
// perhaps
// 
// type List struct {Val int; Next *List}
// 
// could be encoded as
// 
// {"_type": "foo.List", Val: "number", Next: {"_type": "foo.List"}}

//func main() {
//	t := ArrayOf(ObjectOf(
//		map[string]*Type{
//			"field1": SimpleType(String),
//			"field2": ArrayOf(SimpleType(Number)),
//		},
//	))
//	b, err := json.Marshal(t)
//	if err != nil {
//		log.Fatalf("error: %v\n", err)
//	}
//	fmt.Printf("%s\n", b)
//	var x *Type
//	err = json.Unmarshal(b, &x)
//	if err != nil {
//		log.Fatalf("error: %v\n", err)
//	}
//	b, err = json.Marshal(t)
//	if err != nil {
//		log.Fatalf("error: %v\n", err)
//	}
//	fmt.Printf("%s\n", b)
//}

type RequestType struct {
	Params *Type
	Result *Type
}

func RPCInfo(root interface{}) map[string]map[string]RequestType {
	rootInfo := make(map[string]map[string]RequestType)
	rt := reflect.TypeOf(root)
	for i := 0; i < rt.NumMethod(); i++ {
		m := rt.Method(i)
		ret := obtainerType(m)
		if ret == nil {
			continue
		}
		actions := make(map[string]RequestType)
		for i := 0; i < ret.NumMethod(); i++ {
			m := ret.Method(i)
			if r := methodToRequestType(m); r != nil {
				actions[m.Name] = *r
			}
		}
		if len(actions) > 0 {
			rootInfo[m.Name] = actions
		}
	}
	return rootInfo
}

var (
	errorType     = reflect.TypeOf((*error)(nil)).Elem()
	interfaceType = reflect.TypeOf((*interface{})(nil)).Elem()
	stringType    = reflect.TypeOf("")
)

func obtainerType(m reflect.Method) reflect.Type {
	if m.PkgPath != "" {
		return nil
	}
	t := m.Type
	if t.NumIn() != 2 ||
		t.NumOut() != 2 ||
		t.In(1) != stringType ||
		t.Out(1) != errorType {
		return nil
	}
	return t.Out(0)
}

func methodToRequestType(m reflect.Method) *RequestType {
	if m.PkgPath != "" {
		return nil
	}
	t := m.Type
	var rt RequestType
	switch t.NumIn() {
	case 1:
		// Method() ...
		rt.Params = ObjectOf(nil)
	case 2:
		// Method(T) ...
		rt.Params = go2jsonType(t.In(1), false)
	default:
		return nil
	}
	switch {
	case t.NumOut() == 0:
		// Method(...)
		rt.Result = ObjectOf(nil)
	case t.NumOut() == 1 && t.Out(0) == errorType:
		// Method(...) error
		rt.Result = ObjectOf(nil)
	case t.NumOut() == 1:
		// Method(...) R
		rt.Result = go2jsonType(t.Out(0), false)
	case t.NumOut() == 2 && t.Out(1) == errorType:
		// Method(...) (R, error)
		rt.Result = go2jsonType(t.Out(0), false)
	default:
		return nil
	}
	return &rt
}

func go2jsonType(t reflect.Type, canAddr bool) *Type {
	if _, ok := t.MethodByName("MarshalJSON"); ok {
		return CustomType(t.String())
	}
	if canAddr {
		if _, ok := reflect.PtrTo(t).MethodByName("MarshalJSON"); ok {
			return CustomType(t.String())
		}
	}
	switch t.Kind() {
	case reflect.Bool:
		return SimpleType(Bool)
	case reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return SimpleType(Number)
	case reflect.String:
		return SimpleType(String)
	case reflect.Complex64, reflect.Complex128:
		return CustomType("complex")
	case reflect.Chan:
		return CustomType("chan")
	case reflect.Array, reflect.Slice:
		return ArrayOf(go2jsonType(t.Elem(), canAddr || t.Kind() == reflect.Slice))
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			// TODO better error recovery
			panic("unexpected map key kind")
		}
		return MapOf(go2jsonType(t.Elem(), false))
	case reflect.Ptr:
		return NullableOf(go2jsonType(t.Elem(), true))
	case reflect.Struct:
		return ObjectOf(jsonFields(t, canAddr))
	case reflect.UnsafePointer:
		return CustomType("unsafe.Pointer")
	case reflect.Interface:
		return CustomType("any")
	case reflect.Func:
		return CustomType("func")
	}
	panic(fmt.Errorf("unknown kind for type %s", t))
}

func jsonFields(t reflect.Type, canAddr bool) map[string] *Type {
	fields := make(map[string] *Type)
	// TODO anonymous fields
	n := t.NumField()
	for i := 0; i < n; i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		if tag := field.Tag.Get("json"); tag != "" {
			if i := strings.Index(tag, ","); i >= 0 {
				tag = tag[0:i]
			}
			if tag == "-" {
				continue
			}
			if tag != "" {
				field.Name = tag
			}
		}
		// TODO parse json tag
		fields[field.Name] = go2jsonType(field.Type, canAddr)
	}
	return fields
}

type Type struct {
	Kind Kind
	Name string
	Elem *Type
	Fields map[string]*Type
}

func ArrayOf(t *Type) *Type {
	return &Type{
		Kind: Array,
		Elem: t,
	}
}

func MapOf(t *Type) *Type {
	return &Type{
		Kind: Map,
		Elem: t,
	}
}

func NullableOf(t *Type) *Type {
	return &Type{
		Kind: Nullable,
		Elem: t,
	}
}

func ObjectOf(fields map[string]*Type) *Type {
	return &Type{
		Kind: Object,
		Fields: fields,
	}
}

func SimpleType(kind Kind) *Type {
	return &Type{Kind: kind}
}

func CustomType(name string) *Type {
	return &Type{
		Kind: Custom,
		Name: name,
	}
}

type Kind int
const (
	String = iota+1
	Number
	Bool
	Array
	Object
	Custom
	Map
	Nullable
)

var kindStrings = []string{
	String: "string",
	Number: "number",
	Bool: "bool",
	Array: "array",
	Object: "object",
	Map: "map",
	Nullable: "nullable",
	Custom: "custom",
}

func (k Kind) String() string {
	return kindStrings[k]
}

func (t *Type) MarshalJSON() ([]byte, error) {
	var obj interface{}
	switch t.Kind {
	case String, Number, Bool:
		obj = t.Kind.String()
	case Array:
		obj = []*Type{t.Elem}
	case Object:
		if t.Fields == nil {
			obj = struct{}{}
		} else {
			obj = t.Fields
		}
	case Map:
		obj = map[string]*Type{"_map": t.Elem}
	case Nullable:
		obj = map[string]*Type{"_nullable": t.Elem}
	case Custom:
		obj = t.Name
	}
	return json.Marshal(obj)
}

func (t *Type) UnmarshalJSON(b []byte) error {
	switch b[0] {
	case '"':
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		switch s {
		case "string":
			t.Kind = String
		case "number":
			t.Kind = Number
		case "bool":
			t.Kind = Bool
		default:
			t.Kind = Custom
			t.Name = s
		}
		return nil
	case '[':
		var elems []*Type
		if err := json.Unmarshal(b, &elems); err != nil {
			return err
		}
		if len(elems) != 1 {
			return fmt.Errorf("unexpected element count %d in array (need 1)", len(elems))
		}
		t.Kind = Array
		t.Elem = elems[0]
	case '{':
		var fields map[string]*Type
		if err := json.Unmarshal(b, &fields); err != nil {
			return err
		}
		switch {
		case fields["_map"] != nil:
			t.Kind = Map
			t.Elem = fields["_map"]
		case fields["_nullable"] != nil:
			t.Kind = Nullable
			t.Elem = fields["_nullable"]
		default:
			t.Kind = Object
			t.Fields = fields
		}
	default:
		return fmt.Errorf("cannot unmarshal %q into Type", b)
	}
	return nil
}

