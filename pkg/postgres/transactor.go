package postgres

import (
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
)

// RowMapper is a type alias for a function that takes an sqlx.Rows instance and
// within the function is responsible for iterating over all rows and scanning
// the results into some record.
type RowMapper func(*sqlx.Rows) error

// Transactor is a wrapper interface for the underlying sqlx.Tx, which adds some
// machinery for converting named queries into the form Postgres actually wants,
// and handling error rollbacks or commits.
type Transactor interface {
	// CommitOrRollback will finalize a transaction. They will be rolled back on
	// first error in which case this is a noop. If the transaction has not been
	// rolled back it will be committed.
	CommitOrRollback() error

	// Get executes the given sql statement and args, scanning the the result into
	// the given destination.
	Get(dest interface{}, sql string, args interface{}) error

	// Map executes an sqlx.Queryx call and pushes the results row by row to the
	// given RowMapper function.
	Map(sql string, args interface{}, rm RowMapper) error
}

// transactor is the private type that implements the Transactor interface
type transactor struct {
	tx *sqlx.Tx

	sync.Mutex
	finalised    bool
	finalisedErr error
}

// BeginTX is a constructor function that returns a new Transactor ready for
// use. It takes in an sqlx.DB instance and returns either the Transactor or an
// error.
func BeginTX(db *sqlx.DB) (Transactor, error) {
	tx, err := db.Beginx()
	if err != nil {
		return nil, errors.Wrap(err, "failed to start transaction for Transactor")
	}

	return &transactor{
		tx:           tx,
		finalised:    false,
		finalisedErr: nil,
	}, nil
}

// CommitOrRollback keeps track of whether the transaction experienced any
// errors or not. If an error has not occurred then the transaction is rolled
// back, else it is committed.
func (t *transactor) CommitOrRollback() error {
	t.Lock()
	defer t.Unlock()

	if t.finalised {
		return t.finalisedErr
	}

	t.finalised = true
	t.finalisedErr = t.tx.Commit()
	return t.finalisedErr
}

// Get wraps tx.Get and takes as input a destination object (into which the
// result will be scanned), an sql string, as well as some args. As with Exec if
// the sql contains named parameters and our args is a map[string]interface{},
// then the query will be rebound to convert the named args into the type
// postgres is expecting.
func (t *transactor) Get(dest interface{}, sql string, args interface{}) error {
	nsql, nargs, err := t.normalizeSql(sql, args)
	if err != nil {
		return t.rollback(err)
	}

	err = t.tx.Get(dest, nsql, nargs...)
	if err != nil {
		return t.rollback(err)
	}

	return nil
}

// Map is a slightly more complicated function that takes in an sql string and
// args, which are handled as with Exec or Get, but it also takes in a RowMapper
// function which is responsible for iterating through a result set and
// appending each row to a slice in order to return this slice.
func (t *transactor) Map(sql string, args interface{}, rm RowMapper) (err error) {
	var (
		rows *sqlx.Rows
	)

	nsql, nargs, err := t.normalizeSql(sql, args)
	if err != nil {
		return t.rollback(err)
	}

	rows, err = t.tx.Queryx(nsql, nargs...)
	if err != nil {
		return t.rollback(err)
	}

	defer func() {
		if cerr := rows.Close(); err == nil && cerr != nil {
			err = cerr
		}
	}()

	err = rm(rows)
	if err != nil {
		return t.rollback(err)
	}

	return err
}

// normalizeSql is an internal function that converts sql and args into a
// normalised form, converting named args into the default form using sqlx.
func (t *transactor) normalizeSql(sql string, args interface{}) (string, []interface{}, error) {
	var (
		normalizedSql  string
		normalizedArgs []interface{}
		err            error
	)

	switch v := args.(type) {
	case []interface{}:
		normalizedSql = t.tx.Rebind(sql)
		normalizedArgs = v

	default:
		normalizedSql, normalizedArgs, err = t.tx.BindNamed(sql, v)
		if err != nil {
			return "", nil, errors.Wrap(err, "error converting named sql to bindvar version")
		}
	}

	return normalizedSql, normalizedArgs, nil
}

// rollback is a helper function that wraps any error and rolls back the
// transaction.
func (t *transactor) rollback(origErr error) error {
	t.Lock()
	defer t.Unlock()

	if t.finalised {
		return errors.Wrap(origErr, t.finalisedErr.Error())
	}

	t.finalised = true
	rollbackErr := t.tx.Rollback()

	if rollbackErr == nil {
		return origErr
	}

	t.finalisedErr = origErr
	return t.finalisedErr
}
