package query

import (
	"fmt"
	"reflect"
	"strings"
)

// Scanner represents a database row that can scan itself.
type Scanner interface {
	Scan(dest ...interface{}) error
}

// Query constructs a query string for an SQL select statement,
// and returns a Getter that will scan values
// into the fields of dest, which must be a pointer to a struct.
//
// The template holds an SQL statement, which must
// contain an occurrence of the string "$fields".
// The returned query is the result of replacing the
// first occurrence of $fields by a list of the fields in the
// dest struct separated by commas.
// 
// TODO support tags for freer column naming.
func Query(dest interface{}, scan Scanner, template string) (query string, g *Getter) {
	v := reflect.ValueOf(dest)
	t := v.Type()
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		panic(fmt.Errorf("dest must be pointer to struct; got %T", v))
	}
	v, t = v.Elem(), t.Elem()
	if strings.Index(template, "$fields") < 0 {
		panic("template has no $fields")
	}
	var (
		names  []string
		values []interface{}
	)
	for i := 0; i < t.NumField(); i++ {
		names = append(names, t.Field(i).Name)
		values = append(values, v.Field(i).Addr().Interface())
	}
	return strings.Replace(template, "$fields", strings.Join(names, ", "), 1),
		&Getter{
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
