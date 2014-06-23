// +build ignore

package values

// Marshal returns the value encoding of v.
// Marshal  traverses the value recursively.
//
// Boolean values encode as bool.
// Signed integer types encode as int64.
// Unsigned integer types encode as uint64.
// Floating point types encode as float64
// Complex types encode as complex128.
// Structs encode as map[string] interface{}.
// Maps with a string key encode as map[string]interface{}.
// Other maps encode as map[interface{}]interface{}.
// Slices encode as []interface{}
func Marshal(v interface{}) (interface{}, error)
 
// Unmarshal traverses fromv recursively and stores the result in the value pointed to by v.
//
// A value in fromv is stored in v if it can be compatibly assigned to the equivalent
// type in v.
//
// In particular:
//	- an integer or floating point value may be assigned to any other integer or floating point type if
//	it fits without overflow. When assigning a floating point value to an integer, the fractional part is lost.
//
//	- a bool value may be assigned to any bool type
//
//	- a string value may be assigned to any string type.
//
//	- a map value may be assigned to a struct if all its keys are strings; the map
//	values are assigned to respective members of the struct.
//	Map elements with names not in the struct are ignored.
//
//	- a slice value may be assigned to any slice with compatible type, or an array if the number of elements match.
//
//	- a pointer value may be assigned to any type if its element type may be assigned to the type.
//
//	- any value may be assigned to a pointer type if it may be assigned to the element pointed to by the pointer type.
//
func Unmarshal(fromv, v interface{}) error {
	


Marshal traverses the value v recursively. If an encountered value implements the Marshaler interface and is not a nil pointer, Marshal calls its MarshalJSON method to produce JSON. The nil pointer exception is not strictly necessary but mimics a similar, necessary exception in the behavior of UnmarshalJSON.

Otherwise, Marshal uses the following type-dependent default encodings:

Boolean values encode as JSON booleans.

Floating point, integer, and Number values encode as JSON numbers.

String values encode as JSON strings. InvalidUTF8Error will be returned if an invalid UTF-8 sequence is encountered. The angle brackets "<" and ">" are escaped to "\u003c" and "\u003e" to keep some browsers from misinterpreting JSON output as HTML.

Array and slice values encode as JSON arrays, except that []byte encodes as a base64-encoded string, and a nil slice encodes as the null JSON object.

Struct values encode as JSON objects. Each exported struct field becomes a member of the object unless

