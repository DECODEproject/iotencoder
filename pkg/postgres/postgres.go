package postgres

import (
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Open takes as input a connection string for a DB, and returns either a
// sqlx.DB instance or an error.
func Open(connStr string) (*sqlx.DB, error) {
	return sqlx.Open("postgres", connStr)
}
