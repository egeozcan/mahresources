package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

type SQLQuery struct {
	Query        string
	Parameters   []interface{}
	SingleResult bool
}

func getDatabaseHandler(db *sql.DB) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		decoder := json.NewDecoder(r.Body)
		var queries []SQLQuery
		err := decoder.Decode(&queries)

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			fmt.Println(err)
			return
		}

		var rowsList []*sql.Rows
		ctx, err := db.Begin()

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			fmt.Println(err)
			return
		}

		for _, query := range queries {
			fmt.Println(query)
			//return the results of the last query only?
			rows, err := ctx.Query(query.Query, query.Parameters...)
			rowsList = append(rowsList, rows)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				_ = ctx.Rollback()
				fmt.Println(err)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")

		var dataList []interface{}

		for _, rows := range rowsList {
			data, err := SQLToMap(rows)

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				fmt.Println(err)
				return
			}

			dataList = append(dataList, data)
		}

		err = ctx.Commit()

		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		result, err := json.Marshal(dataList)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		_, err = w.Write(result)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	}
}
