package namespace

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestMinDuration(t *testing.T) {
	if got := minDuration(0, 5*time.Second); got != 5*time.Second {
		t.Fatalf("unexpected minDuration zero: %v", got)
	}
	if got := minDuration(3*time.Second, 5*time.Second); got != 3*time.Second {
		t.Fatalf("unexpected minDuration lower: %v", got)
	}
	if got := minDuration(7*time.Second, 5*time.Second); got != 5*time.Second {
		t.Fatalf("unexpected minDuration upper: %v", got)
	}
}

func TestManagerReadWriteAndIdleCollect(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 20 * time.Millisecond})
	defer m.Close()

	dbPath := filepath.Join(t.TempDir(), "ns.db")
	if err := m.WithWrite("n1", dbPath, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT)`)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO t(v) VALUES ('x')`)
		return err
	}); err != nil {
		t.Fatalf("with write: %v", err)
	}

	if err := m.WithRead("n1", dbPath, func(db *sql.DB) error {
		var c int
		return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&c)
	}); err != nil {
		t.Fatalf("with read: %v", err)
	}

	// force idle collection
	time.Sleep(60 * time.Millisecond)
	m.collectIdle()

	// handle should reopen cleanly
	if err := m.WithRead("n1", dbPath, func(db *sql.DB) error {
		var c int
		return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&c)
	}); err != nil {
		t.Fatalf("with read after collect: %v", err)
	}

	if err := m.CloseHandle("n1"); err != nil {
		t.Fatalf("close handle: %v", err)
	}
	if err := m.CloseHandle("n1"); err != nil {
		t.Fatalf("close missing handle: %v", err)
	}
}

func TestCollectIdleSkipsBusyNamespaceWithoutBlocking(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 10 * time.Millisecond})
	defer m.Close()

	dbPath := filepath.Join(t.TempDir(), "busy.db")
	if err := m.WithWrite("busy", dbPath, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v TEXT)`)
		return err
	}); err != nil {
		t.Fatalf("seed namespace: %v", err)
	}

	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.WithWrite("busy", dbPath, func(_ *sql.DB) error {
			close(started)
			time.Sleep(80 * time.Millisecond)
			return nil
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("writer did not start")
	}

	begin := time.Now()
	m.collectIdle()
	if took := time.Since(begin); took > 30*time.Millisecond {
		t.Fatalf("collectIdle blocked on busy namespace: %v", took)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("busy writer returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("busy writer did not finish")
	}
}

// TestCollectIdleDoesNotBlockOtherNamespaces guards against the head-of-line
// blocking pattern where collectIdle holds the manager mutex across a slow
// db.Close() and stalls getOrOpen for unrelated namespaces.
func TestCollectIdleDoesNotBlockOtherNamespaces(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 10 * time.Millisecond})
	defer m.Close()

	idlePath := filepath.Join(t.TempDir(), "idle.db")
	if err := m.WithWrite("idle", idlePath, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)`)
		return err
	}); err != nil {
		t.Fatalf("seed idle namespace: %v", err)
	}

	// Mark "idle" as past the cutoff so collectIdle will evict it.
	time.Sleep(30 * time.Millisecond)

	reaperDone := make(chan struct{})
	go func() {
		m.collectIdle()
		close(reaperDone)
	}()

	// While the reaper runs, opening a *different* namespace must succeed
	// quickly. Even if the reaper's db.Close() is slow, m.mu must be free.
	otherPath := filepath.Join(t.TempDir(), "other.db")
	begin := time.Now()
	err := m.WithWrite("other", otherPath, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY)`)
		return err
	})
	took := time.Since(begin)
	if err != nil {
		t.Fatalf("other namespace open: %v", err)
	}
	if took > 100*time.Millisecond {
		t.Fatalf("other namespace open blocked by reaper: %v", took)
	}
	<-reaperDone
}

// TestEvictionDuringConcurrentReadIsSafe stresses the eviction path against
// concurrent readers/writers. Without the `closed` flag and the WithRead
// retry, an in-flight goroutine could end up using a closed *sql.DB.
func TestEvictionDuringConcurrentReadIsSafe(t *testing.T) {
	m := NewManager(Config{IdleTimeout: 5 * time.Millisecond})
	defer m.Close()

	dbPath := filepath.Join(t.TempDir(), "stress.db")
	if err := m.WithWrite("stress", dbPath, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS t (id INTEGER PRIMARY KEY, v INTEGER)`)
		if err != nil {
			return err
		}
		_, err = db.Exec(`INSERT INTO t(v) VALUES (1)`)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	stop := make(chan struct{})
	var failures atomic.Int64
	var ops atomic.Int64
	var wg sync.WaitGroup

	// Several readers/writers continuously hammer the namespace.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				err := m.WithRead("stress", dbPath, func(db *sql.DB) error {
					var n int
					return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&n)
				})
				ops.Add(1)
				if err != nil {
					failures.Add(1)
				}
			}
		}()
	}

	// Run reaper aggressively in parallel.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			m.collectIdle()
			time.Sleep(2 * time.Millisecond)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	close(stop)
	wg.Wait()

	if ops.Load() == 0 {
		t.Fatal("no operations executed")
	}
	if failures.Load() > 0 {
		t.Fatalf("got %d failed reads under concurrent eviction (out of %d ops)", failures.Load(), ops.Load())
	}
}

func TestManagerBoundsRetainedHandles(t *testing.T) {
	m := NewManager(Config{MaxRetained: 2})
	defer m.Close()

	dir := t.TempDir()
	for _, name := range []string{"one", "two", "three"} {
		path := filepath.Join(dir, name+".db")
		if err := m.WithWrite(name, path, func(db *sql.DB) error {
			_, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
			return err
		}); err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
	}
	if got := m.OpenCount(); got > 2 {
		t.Fatalf("retained %d handles, want at most 2", got)
	}
}

func TestManagerDoesNotFailUnderCapacityChurn(t *testing.T) {
	m := NewManager(Config{MaxRetained: 4})
	defer m.Close()

	const namespaceCount = 32
	dir := t.TempDir()
	paths := make([]string, namespaceCount)
	for index := range paths {
		paths[index] = filepath.Join(dir, fmt.Sprintf("churn-%d.db", index))
		if err := m.WithWrite(fmt.Sprintf("churn-%d", index), paths[index], func(db *sql.DB) error {
			_, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
			return err
		}); err != nil {
			t.Fatalf("seed namespace %d: %v", index, err)
		}
	}

	var failures atomic.Int64
	var wg sync.WaitGroup
	for worker := 0; worker < 32; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for operation := 0; operation < 50; operation++ {
				index := (worker + operation) % namespaceCount
				name := fmt.Sprintf("churn-%d", index)
				if err := m.WithRead(name, paths[index], func(db *sql.DB) error {
					var count int
					return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&count)
				}); err != nil {
					failures.Add(1)
				}
			}
		}(worker)
	}
	wg.Wait()

	if got := failures.Load(); got != 0 {
		t.Fatalf("capacity churn produced %d failed reads", got)
	}
	if got := m.OpenCount(); got > 4 {
		t.Fatalf("capacity churn retained %d handles, want at most 4", got)
	}
}

func TestManagerConfiguresReadConnectionLimit(t *testing.T) {
	m := NewManager(Config{ReadConns: 3})
	defer m.Close()

	path := filepath.Join(t.TempDir(), "read-pool.db")
	if err := m.WithRead("read-pool", path, func(db *sql.DB) error {
		if got := db.Stats().MaxOpenConnections; got != 3 {
			t.Fatalf("max open read connections = %d, want 3", got)
		}
		return nil
	}); err != nil {
		t.Fatalf("open read pool: %v", err)
	}
}

func TestManagerCoalescesConcurrentOpen(t *testing.T) {
	var opens atomic.Int64
	var closes atomic.Int64
	m := NewManager(Config{
		OnReadOpen: func(_, _ string, _ *sql.DB) error {
			opens.Add(1)
			time.Sleep(20 * time.Millisecond)
			return nil
		},
		OnReadClose: func(_, _ string, _ *sql.DB) {
			closes.Add(1)
		},
	})

	path := filepath.Join(t.TempDir(), "coalesced.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- m.WithRead("shared", path, func(db *sql.DB) error {
				var count int
				return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&count)
			})
		}()
	}
	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent read: %v", err)
		}
	}
	if got := opens.Load(); got != 1 {
		t.Fatalf("opened namespace %d times, want 1", got)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("close manager: %v", err)
	}
	if got := closes.Load(); got != 1 {
		t.Fatalf("closed namespace %d times, want 1", got)
	}
}

func TestManagerOpensReadAndWritePoolsLazily(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	var readOpens, writeOpens, readCloses, writeCloses atomic.Int64
	m := NewManager(Config{
		OnReadOpen: func(_, _ string, _ *sql.DB) error {
			readOpens.Add(1)
			return nil
		},
		OnWriteOpen: func(_, _ string, _ *sql.DB) error {
			writeOpens.Add(1)
			return nil
		},
		OnReadClose: func(_, _ string, _ *sql.DB) {
			readCloses.Add(1)
		},
		OnWriteClose: func(_, _ string, _ *sql.DB) {
			writeCloses.Add(1)
		},
	})
	if err := m.WithRead("read-only", path, func(db *sql.DB) error {
		var count int
		return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if readOpens.Load() != 1 || writeOpens.Load() != 0 {
		t.Fatalf("read opened read=%d write=%d pools", readOpens.Load(), writeOpens.Load())
	}
	if err := m.CloseHandle("read-only"); err != nil {
		t.Fatal(err)
	}
	if readCloses.Load() != 1 || writeCloses.Load() != 0 {
		t.Fatalf("read closed read=%d write=%d pools", readCloses.Load(), writeCloses.Load())
	}

	if err := m.WithWrite("write-only", path, func(db *sql.DB) error {
		_, err := db.Exec(`INSERT INTO t DEFAULT VALUES`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if readOpens.Load() != 1 || writeOpens.Load() != 1 {
		t.Fatalf("write opened read=%d write=%d pools", readOpens.Load(), writeOpens.Load())
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if readCloses.Load() != 1 || writeCloses.Load() != 1 {
		t.Fatalf("close counts read=%d write=%d", readCloses.Load(), writeCloses.Load())
	}
}

func TestReadCanRunDuringWrite(t *testing.T) {
	m := NewManager(Config{})
	defer m.Close()

	path := filepath.Join(t.TempDir(), "parallel.db")
	if err := m.WithWrite("parallel", path, func(db *sql.DB) error {
		_, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY)`)
		return err
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- m.WithWrite("parallel", path, func(*sql.DB) error {
			close(started)
			<-release
			return nil
		})
	}()
	<-started

	begin := time.Now()
	if err := m.WithRead("parallel", path, func(db *sql.DB) error {
		var count int
		return db.QueryRow(`SELECT COUNT(*) FROM t`).Scan(&count)
	}); err != nil {
		t.Fatalf("read during write: %v", err)
	}
	if took := time.Since(begin); took > 100*time.Millisecond {
		t.Fatalf("read blocked behind writer for %v", took)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("writer: %v", err)
	}
}
