package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/store/sqlite"
)

// ExecSQL executes one bound statement and commits before publishing changes.
func (s *Service) ExecSQL(ctx context.Context, ns string, st sqlite.TransactionStatement) (sqlite.ExecResult, error) {
	st.Mode = "exec"
	out, err := s.Transaction(ctx, ns, sqlite.TransactionRequest{Statements: []sqlite.TransactionStatement{st}})
	if err != nil {
		return sqlite.ExecResult{}, err
	}
	return *out.Results[0].ExecResult, nil
}

// Transaction serializes SQL with existing writers and publishes only committed changes.
func (s *Service) Transaction(ctx context.Context, ns string, req sqlite.TransactionRequest) (sqlite.TransactionResult, error) {
	ctx, cancel := context.WithTimeout(ctx, sqlite.MaxSQLDuration)
	defer cancel()
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return sqlite.TransactionResult{}, err
	}
	var out sqlite.TransactionResult
	var changes []sqlite.SQLChange
	err = s.ns.WithWriteContext(ctx, ns, path, func(db *sql.DB) error {
		cfg := defaultNamespaceConfig()
		if _, err := sqlite.GetMeta(db, "namespace_config", "self", &cfg); err != nil {
			return err
		}
		var err error
		out, changes, err = sqlite.ExecuteTransaction(ctx, path, cfg, req)
		if err == nil && len(changes) > 0 {
			sqlite.InvalidateRuntime(db)
		}
		return err
	})
	if err != nil {
		return sqlite.TransactionResult{}, mapSQLError(err)
	}
	for _, change := range changes {
		action := "invalidate"
		if change.Schema {
			action = "schema_change"
		}
		s.broker.Publish(ns, domain.SSEEvent{
			Namespace: ns, Table: change.Table, Action: action,
			Detail: map[string]any{"source": "sql"}, Meta: req.Meta,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		})
		s.publishViewInvalidations(ns, change.Table, action)
	}
	return out, nil
}

func mapSQLError(err error) error {
	var statement *sqlite.StatementError
	index := (*int)(nil)
	if errors.As(err, &statement) {
		index = &statement.Index
		err = statement.Err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		err = domain.NewError(domain.ErrSQLTimeout, 408, "SQL request deadline exceeded or request canceled")
	}
	mapped := mapErr(err)
	var e *domain.Error
	if errors.As(mapped, &e) && index != nil {
		copy := *e
		copy.StatementIndex = index
		copy.Message = fmt.Sprintf("statement %d: %s", *index, e.Message)
		return &copy
	}
	return mapped
}
