package namespace

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestWriterWaitCanBeCanceled(t *testing.T) {
	m := NewManager(Config{})
	defer m.Close()
	path := filepath.Join(t.TempDir(), "writer.db")
	locked := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.WithWrite("writer", path, func(*sql.DB) error { close(locked); <-release; return nil })
	}()
	<-locked
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := m.WithWriteContext(ctx, "writer", path, func(*sql.DB) error { t.Error("canceled callback ran"); return nil })
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("writer wait: %v", err)
	}
	if err := m.WithWrite("writer", path, func(*sql.DB) error { return nil }); err != nil {
		t.Fatalf("writer recovery: %v", err)
	}
}
