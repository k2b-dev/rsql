package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/store/sqlite"
)

func TestQueryWaitUsesNamespaceReaderSlots(t *testing.T) {
	svc := newTestService(t)
	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{Name: "readers"}); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- svc.WithNamespaceRead("readers", func(*sql.DB) error {
			close(locked)
			<-release
			return nil
		})
	}()
	<-locked
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := svc.QueryContext(ctx, "readers", sqlite.QueryRequest{SQL: `SELECT 1`})
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	var e *domain.Error
	if !errors.As(err, &e) || e.Code != domain.ErrSQLTimeout {
		t.Fatalf("isolated SQL reader bypassed the shared read limit: %v", err)
	}
	if _, err := svc.Query("readers", sqlite.QueryRequest{SQL: `SELECT 1`}); err != nil {
		t.Fatalf("reader recovery: %v", err)
	}
}
