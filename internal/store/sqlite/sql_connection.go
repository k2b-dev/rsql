package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"

	"github.com/k2b-dev/rsql/internal/domain"
	driver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Raw SQL never runs on a pooled CRUD connection. Authorizer and limits belong
// to this request alone, including the legacy read-only route.
func openSQLConnection(ctx context.Context, path string, cfg domain.NamespaceConfig, readOnly bool) (*sql.DB, *sql.Conn, *sqlPolicy, error) {
	file := &url.URL{Scheme: "file", Path: path}
	dsn := file.String() + "?_defensive=true&_pragma=trusted_schema(OFF)&_pragma=temp_store(FILE)&_pragma=busy_timeout(1000)"
	if readOnly {
		dsn += "&mode=ro&_pragma=query_only(ON)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, nil, err
	}
	fail := func(err error) (*sql.DB, *sql.Conn, *sqlPolicy, error) {
		_ = db.Close()
		return nil, nil, nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if !readOnly {
		if cfg.BusyTimeout <= 0 || cfg.BusyTimeout > 1000 {
			cfg.BusyTimeout = 1000
		}
		if err := ApplyNamespaceConfigContext(ctx, db, cfg); err != nil {
			return fail(err)
		}
	}
	conn, err := db.Conn(ctx)
	if err != nil {
		return fail(err)
	}
	failConn := func(err error) (*sql.DB, *sql.Conn, *sqlPolicy, error) {
		_ = conn.Close()
		return fail(err)
	}
	for _, limit := range [][2]int{
		{sqlite3.SQLITE_LIMIT_LENGTH, MaxSQLResultBytes},
		{sqlite3.SQLITE_LIMIT_SQL_LENGTH, MaxSQLBytes},
		{sqlite3.SQLITE_LIMIT_VARIABLE_NUMBER, MaxSQLParams},
		{sqlite3.SQLITE_LIMIT_ATTACHED, 0},
		{sqlite3.SQLITE_LIMIT_EXPR_DEPTH, 100},
		{sqlite3.SQLITE_LIMIT_COLUMN, 256},
		{sqlite3.SQLITE_LIMIT_COMPOUND_SELECT, 100},
		{sqlite3.SQLITE_LIMIT_LIKE_PATTERN_LENGTH, 4096},
		{sqlite3.SQLITE_LIMIT_VDBE_OP, 100000},
	} {
		if _, err := driver.Limit(conn, limit[0], limit[1]); err != nil {
			return failConn(err)
		}
	}
	policy, err := loadSQLPolicy(ctx, conn)
	if err != nil {
		return failConn(err)
	}
	policy.readOnly = readOnly
	if err := setSQLAuthorizer(conn, policy.authorize); err != nil {
		return failConn(err)
	}
	return db, conn, policy, nil
}

func setSQLAuthorizer(conn *sql.Conn, fn driver.AuthorizerFn) error {
	return conn.Raw(func(c any) error {
		a, ok := c.(driver.AuthorizerRegisterer)
		if !ok {
			return fmt.Errorf("SQLite authorizer unavailable")
		}
		return a.RegisterAuthorizer(fn)
	})
}

// ColumnInfo prepares without stepping. Cap every result value before SQLite
// allocates output registers or the driver copies them. Reserve worst-case JSON
// escaping for every column, so wide rows cannot multiply the memory budget.
func boundSQLResult(conn *sql.Conn, query string, remaining int) error {
	if _, err := driver.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, MaxSQLResultBytes); err != nil {
		return err
	}
	var columns []driver.ColumnInfo
	err := conn.Raw(func(c any) error {
		ci, ok := c.(interface {
			ColumnInfo(string) ([]driver.ColumnInfo, error)
		})
		if !ok {
			return fmt.Errorf("SQLite column metadata unavailable")
		}
		var err error
		columns, err = ci.ColumnInfo(query)
		return err
	})
	if err != nil {
		return err
	}
	limit := MaxSQLResultBytes
	if len(columns) > 0 {
		// Keys are bounded by the 64 KiB SQL text / schema budget; scan checks
		// their exact encoded size before retaining a row.
		limit = max(1, remaining/(6*len(columns)))
	}
	_, err = driver.Limit(conn, sqlite3.SQLITE_LIMIT_LENGTH, limit)
	return err
}
