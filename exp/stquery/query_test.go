package stquery_test

import (
	"code.google.com/p/rog-go/exp/stquery"
	"fmt"
	"reflect"
	"strconv"
	"testing"
)

type Accounting struct {
	JobNumber  int
	TaskNumber int `stquery:"j_task_nunber"`
	PETaskId   string
	Name       string
	CPU        float64
}

var row = []string{
	"0", "1", "hello world", "name", "3.4546",
}

var accountingValue = Accounting{
	0, 1, "hello world", "name", 3.4546,
}

func TestQuery(t *testing.T) {
	var acct Accounting

	q := stquery.Statement(&acct, "select $fields from somewhere")
	if q != "select JobNumber, j_task_nunber, PETaskId, Name, CPU from somewhere" {
		t.Errorf("invalid query generated; got %q", q)
	}
	scan := newRowScanner(row)

	getter := stquery.NewGetter(&acct, scan)
	err := getter.Get()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(acct, accountingValue) {
		t.Fatalf("got %+v", acct)
	}
}

type rowScanner struct {
	row []string
}

func newRowScanner(row []string) stquery.Scanner {
	return &rowScanner{row}
}

func (scan *rowScanner) Scan(values ...interface{}) error {
	if len(values) != len(scan.row) {
		return fmt.Errorf("mismatched row and scan values")
	}
	for i, v := range values {
		switch v := v.(type) {
		case *int:
			n, err := strconv.Atoi(row[i])
			if err != nil {
				return err
			}
			*v = n
		case *string:
			*v = row[i]
		case *float64:
			f, err := strconv.ParseFloat(row[i], 64)
			if err != nil {
				return err
			}
			*v = f
		default:
			return fmt.Errorf("unsupported type %T", v)
		}
	}
	return nil
}
