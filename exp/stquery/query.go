package stquery

import (
	"fmt"
	"reflect"
	"strings"
)

// Scanner represents a database row that can scan itself.
type Scanner interface {
	Scan(dest ...interface{}) error
}

// Statement returns an SQL query statement that selects columns based
// on the names of the fields in dest, which must be a pointer
// to a struct. If a field has the "stquery" tag, its value holds
// the name that will be used.
//
// The given template string must contain an occurrence of "$fields",
// which will be replaced with a comma-separated list of the names.
func Statement(dest interface{}, template string) (query string) {
	v := reflect.ValueOf(dest)
	t := v.Type()
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("dest must be pointer to struct; got %T", v))
	}
	t = t.Elem()
	if strings.Index(template, "$fields") < 0 {
		panic("template has no $fields")
	}
	names := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := f.Name
		if tag := f.Tag.Get("stquery"); tag != "" {
			name = tag
		}
		names = append(names, name)
	}
	return strings.Replace(template, "$fields", strings.Join(names, ", "), 1)
}

// NewGetter returns a Getter that uses the given scanner
// to read values into the fields in dest, which must
// be a pointer to a struct.
func NewGetter(dest interface{}, scan Scanner) *Getter {
	v := reflect.ValueOf(dest)
	t := v.Type()
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("dest must be pointer to struct; got %T", v))
	}
	v = v.Elem()
	values := make([]interface{}, 0, v.NumField())
	for i := 0; i < v.NumField(); i++ {
		values = append(values, v.Field(i).Addr().Interface())
	}
	return &Getter{
		scan:   scan,
		values: values,
	}
}

// Getter represents a value that can fetch itself by calling
// Scan on an underlying Scanner.
type Getter struct {
	scan   Scanner
	values []interface{}
}

// Get gets a set of values by calling Scan, and stores them
// in fields of the destination value.
func (g *Getter) Get() error {
	return g.scan.Scan(g.values...)
}
