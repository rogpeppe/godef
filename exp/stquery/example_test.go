package stquery_test

import (
	"code.google.com/p/rog-go/exp/stquery"
	"database/sql"
	"fmt"
	"log"
	"testing"
)

func ExampleGetter(t *testing.T) {
	db := openDatabase()
	var row struct {
		Name string
		Age  int
	}
	rows, err := db.Query(stquery.Statement(&row, `SELECT $fields
		FROM sge_job
		WHERE j_job_number = $1 AND j_task_number = -1
		ORDER BY j_job_number DESC`))
	if err != nil {
		log.Fatal(err)
	}
	getter := stquery.NewGetter(&row, rows)
	for rows.Next() {
		if err := getter.Get(); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Printf("got row %#v", row)
}

func openDatabase() *sql.DB {
	return nil
}
