/*
2019 © Postgres.ai
*/

package querier

import (
	"database/sql"

	"gitlab.com/postgres-ai/database-lab/pkg/log"
)

const QUERY_EXPLAIN = "EXPLAIN (FORMAT TEXT) "
const QUERY_EXPLAIN_ANALYZE = "EXPLAIN (ANALYZE, COSTS, VERBOSE, BUFFERS, FORMAT JSON) "

func DBExec(connStr string, query string) error {
	_, err := runQuery(connStr, query, true)
	return err
}

func DBExplain(connStr string, query string) (string, error) {
	return runQuery(connStr, QUERY_EXPLAIN+query, false)
}

func DBExplainAnalyze(connStr string, query string) (string, error) {
	return runQuery(connStr, QUERY_EXPLAIN_ANALYZE+query, false)
}

func runQuery(connStr string, query string, omitResp bool) (string, error) {
	log.Dbg("DB query:", query)

	// TODO(anatoly): Retry mechanic.
	var result = ""

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Err("DB connection:", err)
		return "", err
	}
	defer db.Close()


	rows, err := db.Query(query)
	if err != nil {
		log.Err("DB query:", err)
		return "", err
	}
	defer rows.Close()

	if !omitResp {
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				log.Err("DB query traversal:", err)
				return s, err
			}
			result += s + "\n"
		}
		if err := rows.Err(); err != nil {
			log.Err("DB query traversal:", err)
			return result, err
		}
	}

	return result, nil
}
