package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/store/control"
)

func TestStartupDoesNotOpenOrScanTenantDatabases(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	path := control.PathFor(dir, "broken")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not a sqlite database"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.registry.Create("broken", path); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatalf("startup touched broken tenant database: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := reopened.ns.OpenCount(); got != 0 {
		t.Fatalf("startup retained %d namespace handles", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("startup modified namespace file: %v", err)
	}
}

func TestMissingNamespaceFileFailsLazily(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateNamespace(domain.NamespaceDefinition{
		Name:   "missing",
		Config: defaultNamespaceConfig(),
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ns.CloseHandle("missing"); err != nil {
		t.Fatal(err)
	}
	path := control.PathFor(dir, "missing")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if _, err := reopened.registry.Get("missing"); err != nil {
		t.Fatalf("startup mutated registry: %v", err)
	}
	if _, err := reopened.GetNamespaceConfig("missing"); err == nil {
		t.Fatal("missing namespace file remained usable")
	}
	if _, err := reopened.registry.Get("missing"); err != nil {
		t.Fatalf("data-plane miss mutated registry: %v", err)
	}
}

func TestFirstWriteInitializesPartialNamespace(t *testing.T) {
	dir := t.TempDir()
	svc, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	path := control.PathFor(dir, "partial")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := svc.registry.Create("partial", path); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.CreateTableOrView("partial", domain.TableCreateRequest{
		Type:    "table",
		Name:    "items",
		Columns: []domain.ColumnDefinition{{Name: "value", Type: "text"}},
	}); err != nil {
		t.Fatalf("first write did not initialize namespace: %v", err)
	}
}

func TestStartupLeavesTransientDirectoriesUntouched(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "exports", "stale.db")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc, err := New(dir, 0, 128, 1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	if _, err := os.Stat(stale); err != nil {
		t.Fatalf("startup scanned or removed transient file: %v", err)
	}
}

func TestUnknownNamespaceDoesNotCreateDatabase(t *testing.T) {
	svc := newTestService(t)
	path := control.PathFor(svc.dataDir, "unknown")
	_, err := svc.GetRow("unknown", "items", 1)
	if err == nil {
		t.Fatal("unknown namespace read succeeded")
	}
	var domainErr *domain.Error
	if !errors.As(err, &domainErr) || domainErr.HTTPStatus != 404 {
		t.Fatalf("expected namespace 404, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown namespace created a database: %v", err)
	}
}
