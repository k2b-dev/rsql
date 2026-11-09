package sqlite

import (
	"context"
	"database/sql"

	"github.com/k2b-dev/rsql/internal/domain"
	driver "modernc.org/sqlite"
)

// Compile the view body without executing it, under the same read policy used
// by raw SQL. Framing also ensures CREATE VIEW cannot carry another statement.
// The caller holds the schema transaction on conn until CREATE VIEW completes.
func validateViewSQL(conn *sql.Conn, query string) error {
	if err := validateSQLStatement(TransactionStatement{SQL: query, Mode: "query"}); err != nil {
		return err
	}
	policy, err := loadSQLPolicy(context.Background(), conn)
	if err != nil {
		return err
	}
	policy.readOnly = true
	if err := setSQLAuthorizer(conn, policy.authorize); err != nil {
		return err
	}
	defer func() { _ = setSQLAuthorizer(conn, nil) }()
	err = conn.Raw(func(c any) error {
		ci, ok := c.(interface {
			ColumnInfo(string) ([]driver.ColumnInfo, error)
		})
		if !ok {
			return domain.NewError(domain.ErrSQLForbidden, 400, "SQLite column metadata unavailable")
		}
		_, err := ci.ColumnInfo(query)
		return err
	})
	return policy.sqlError(err)
}
