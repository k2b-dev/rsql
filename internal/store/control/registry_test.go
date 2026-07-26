package control

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistryCRUD(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer r.Close()

	p := PathFor(dir, "workspace")
	relative, err := filepath.Rel(filepath.Join(dir, "namespaces"), p)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(relative, string(filepath.Separator))
	if len(parts) != 3 || len(parts[0]) != 2 || len(parts[1]) != 2 || len(strings.TrimSuffix(parts[2], ".db")) != 64 {
		t.Fatalf("unexpected sharded path: %s", p)
	}
	if PathFor(dir, "workspace") != p {
		t.Fatal("namespace path is not deterministic")
	}
	if PathFor(dir, "other") == p {
		t.Fatal("different namespaces resolved to the same path")
	}

	if err := r.Create("workspace", p); err != nil {
		t.Fatalf("create namespace record: %v", err)
	}

	rec, err := r.Get("workspace")
	if err != nil {
		t.Fatalf("get namespace record: %v", err)
	}
	if rec.Name != "workspace" {
		t.Fatalf("unexpected record: %#v", rec)
	}

	list, err := r.List()
	if err != nil {
		t.Fatalf("list namespace records: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("unexpected list len: %d", len(list))
	}

	if err := r.Delete("workspace"); err != nil {
		t.Fatalf("delete namespace record: %v", err)
	}
	if err := r.Delete("workspace"); err == nil {
		t.Fatal("expected sql.ErrNoRows for second delete")
	}

	if _, err := r.Get("workspace"); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestRegistryOpenError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "as-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := Open(file); err == nil {
		t.Fatal("expected open error when data dir is a file")
	}
}

func TestRegistryCloseNil(t *testing.T) {
	var r *Registry
	if err := r.Close(); err != nil {
		t.Fatalf("close nil registry: %v", err)
	}
	r = &Registry{}
	if err := r.Close(); err != nil {
		t.Fatalf("close empty registry: %v", err)
	}
}

func TestRegistryInitSchemaError(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer r.Close()

	_, err = r.db.Exec(`DROP TABLE namespaces`)
	if err != nil {
		t.Fatalf("drop namespaces: %v", err)
	}
	if err := r.Delete("x"); err == nil {
		t.Fatal("expected error after dropping table")
	}

	if _, err := r.Get("x"); err == nil {
		t.Fatal("expected get error after dropping table")
	}
	if _, err := r.List(); err == nil {
		t.Fatal("expected list error after dropping table")
	}
	if err := r.Create("x", "y"); err == nil {
		t.Fatal("expected create error after dropping table")
	}

	// restore schema for clean close.
	if err := initSchema(r.db); err != nil {
		t.Fatalf("re-init schema: %v", err)
	}
}

func TestRegistryDeleteNoRowsType(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer r.Close()

	err = r.Delete("missing")
	if err == nil {
		t.Fatal("expected delete error for missing namespace")
	}
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

// TestRegistryConcurrentCreates verifies that parallel Create calls all
// succeed when targeting different names. Without WAL + busy_timeout this
// races on the SQLite exclusive write lock and most callers receive
// SQLITE_BUSY instantly.
func TestRegistryConcurrentCreates(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer r.Close()

	const n = 20
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			name := "ns_" + string(rune('a'+idx))
			errs <- r.Create(name, PathFor(dir, name))
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent create failed: %v", err)
		}
	}

	list, err := r.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != n {
		t.Fatalf("expected %d namespaces, got %d", n, len(list))
	}
}

func TestRegistryListPage(t *testing.T) {
	dir := t.TempDir()
	registry, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = registry.Close() })

	for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
		if err := registry.Create(name, PathFor(dir, name)); err != nil {
			t.Fatal(err)
		}
	}

	first, cursor, err := registry.ListPage(2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || first[0].Name != "alpha" || first[1].Name != "bravo" || cursor != "bravo" {
		t.Fatalf("unexpected first page: records=%#v cursor=%q", first, cursor)
	}
	second, cursor, err := registry.ListPage(2, cursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 2 || second[0].Name != "charlie" || second[1].Name != "delta" || cursor != "" {
		t.Fatalf("unexpected second page: records=%#v cursor=%q", second, cursor)
	}
}
