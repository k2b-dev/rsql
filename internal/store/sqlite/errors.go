package sqlite

import (
	"errors"
	"fmt"

	sqlitedriver "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// ErrValidation marks an error from row-value validation (type, range,
// pattern, options, NOT NULL, formula). The service layer translates it
// to HTTP 400 via errors.Is so the wire response stays stable even if the
// underlying message text is reworded.
var ErrValidation = errors.New("validation_failed")

// ErrQuotaTooSmall marks a quota that cannot contain the current database.
var ErrQuotaTooSmall = errors.New("quota_too_small")

// validationErrf wraps the sentinel with a per-site message. Callers use
// it as a drop-in for fmt.Errorf when the cause is invalid input data.
func validationErrf(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrValidation}, a...)...)
}

// IsFullError reports whether SQLite rejected a write because the database
// reached its configured page limit.
func IsFullError(err error) bool {
	var sqliteErr *sqlitedriver.Error
	return errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == sqlite3.SQLITE_FULL
}

// IsBusyError reports SQLite busy and locked errors.
func IsBusyError(err error) bool {
	var sqliteErr *sqlitedriver.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	code := sqliteErr.Code() & 0xff
	return code == sqlite3.SQLITE_BUSY || code == sqlite3.SQLITE_LOCKED
}
