package namespace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

func TestReaderSlotsRespectConfigurationAndCancellation(t *testing.T) {
	for _, slots := range []int{1, 2} {
		t.Run(fmt.Sprint(slots), func(t *testing.T) {
			m := NewManager(Config{ReadConns: slots})
			defer m.Close()
			path := filepath.Join(t.TempDir(), "readers.db")
			locked := make(chan struct{}, slots)
			release := make(chan struct{})
			done := make(chan error, slots)
			for range slots {
				go func() {
					done <- m.WithRead("readers", path, func(*sql.DB) error {
						locked <- struct{}{}
						<-release
						return nil
					})
				}()
			}
			for range slots {
				<-locked
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := m.WithReadContext(ctx, "readers", path, func(*sql.DB) error {
				t.Error("reader exceeded configured slots")
				return nil
			})
			close(release)
			for range slots {
				if err := <-done; err != nil {
					t.Fatal(err)
				}
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("reader wait: %v", err)
			}
			if err := m.WithRead("readers", path, func(*sql.DB) error { return nil }); err != nil {
				t.Fatalf("reader recovery: %v", err)
			}
		})
	}
}
