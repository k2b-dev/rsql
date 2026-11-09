package namespace

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Config configures namespace manager lifecycle behavior.
type Config struct {
	IdleTimeout  time.Duration
	MaxRetained  int
	ReadConns    int
	BeforeOpen   func(name, path string) error
	OnReadOpen   func(name, path string, db *sql.DB) error
	OnWriteOpen  func(name, path string, db *sql.DB) error
	OnReadClose  func(name, path string, db *sql.DB)
	OnWriteClose func(name, path string, db *sql.DB)
	OnWriteWait  func(name string, wait time.Duration)
}

type openCall struct {
	done chan struct{}
	err  error
}

type handle struct {
	name       string
	path       string
	readDB     *sql.DB
	writeDB    *sql.DB
	poolsMu    sync.Mutex
	lifecycle  sync.RWMutex
	writeSlot  chan struct{}
	lastUsed   int64
	referenced atomic.Bool
	element    *list.Element
}

// Manager manages namespace DB handles and per-namespace locks.
type Manager struct {
	mu        sync.RWMutex
	handles   map[string]*handle
	opening   map[string]*openCall
	order     *list.List
	cfg       Config
	closed    chan struct{}
	closeOnce sync.Once
	shutdown  atomic.Bool
	retained  atomic.Int64
}

// NewManager creates a namespace manager.
func NewManager(cfg Config) *Manager {
	if cfg.MaxRetained <= 0 {
		cfg.MaxRetained = 128
	}
	if cfg.ReadConns <= 0 {
		cfg.ReadConns = 1
	}
	m := &Manager{
		handles: make(map[string]*handle),
		opening: make(map[string]*openCall),
		order:   list.New(),
		cfg:     cfg,
		closed:  make(chan struct{}),
	}
	go m.reaper()
	return m
}

func (m *Manager) reaper() {
	if m.cfg.IdleTimeout <= 0 {
		return
	}
	ticker := time.NewTicker(minDuration(m.cfg.IdleTimeout/2, 30*time.Second))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.collectIdle()
		case <-m.closed:
			return
		}
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a <= 0 {
		return b
	}
	if a < b {
		return a
	}
	return b
}

func (m *Manager) collectIdle() {
	if m.cfg.IdleTimeout <= 0 {
		return
	}
	cutoff := time.Now().Add(-m.cfg.IdleTimeout)

	// Claim a bounded number of idle handles under m.mu. Lifecycle locks stay
	// held through close so requests already pinned to a handle can finish.
	type victim struct {
		name string
		h    *handle
	}
	var victims []victim

	m.mu.Lock()
	scans := min(m.order.Len(), 256)
	for range scans {
		element := m.order.Front()
		if element == nil {
			break
		}
		h := element.Value.(*handle)
		if !h.lastUsedAt().Before(cutoff) {
			m.order.MoveToBack(element)
			continue
		}
		if !h.lifecycle.TryLock() {
			m.order.MoveToBack(element)
			continue
		}
		if !h.lastUsedAt().Before(cutoff) {
			h.lifecycle.Unlock()
			m.order.MoveToBack(element)
			continue
		}
		delete(m.handles, h.name)
		m.retained.Add(-1)
		m.order.Remove(element)
		h.element = nil
		victims = append(victims, victim{h.name, h})
	}
	m.mu.Unlock()

	// Close phase: outside m.mu so getOrOpen calls for unrelated namespaces
	// stay responsive even if Close() takes time draining connections.
	for _, v := range victims {
		m.closeHandle(v.name, v.h)
		v.h.lifecycle.Unlock()
	}
}

// Close closes all open namespace handles. Safe to call multiple times;
// subsequent calls are no-ops. After Close, getOrOpen rejects new work.
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.shutdown.Store(true)
		close(m.closed)

		m.mu.RLock()
		opening := make([]*openCall, 0, len(m.opening))
		for _, call := range m.opening {
			opening = append(opening, call)
		}
		m.mu.RUnlock()
		for _, call := range opening {
			<-call.done
		}

		m.mu.Lock()
		handles := make(map[string]*handle, len(m.handles))
		for name, h := range m.handles {
			handles[name] = h
			delete(m.handles, name)
			h.element = nil
		}
		m.order.Init()
		m.retained.Store(0)
		m.mu.Unlock()

		for name, h := range handles {
			h.lifecycle.Lock()
			m.closeHandle(name, h)
			h.lifecycle.Unlock()
		}
	})
	return nil
}

// getOrOpenPinned returns a handle with its lifecycle read lock held. Pinning
// while the manager lock still protects the map closes the race where a newly
// opened handle could be evicted before its request started using it.
func (m *Manager) getOrOpenPinned(name, path string) (*handle, error) {
	for {
		if m.shutdown.Load() {
			return nil, fmt.Errorf("namespace manager is shut down")
		}
		m.mu.RLock()
		if h, ok := m.handles[name]; ok {
			h.lifecycle.RLock()
			h.touch()
			m.mu.RUnlock()
			return h, nil
		}
		m.mu.RUnlock()

		m.mu.Lock()
		if m.shutdown.Load() {
			m.mu.Unlock()
			return nil, fmt.Errorf("namespace manager is shut down")
		}
		if h, ok := m.handles[name]; ok {
			h.lifecycle.RLock()
			h.touch()
			m.mu.Unlock()
			return h, nil
		}
		if call, ok := m.opening[name]; ok {
			m.mu.Unlock()
			<-call.done
			if call.err != nil {
				return nil, call.err
			}
			continue
		}
		call := &openCall{done: make(chan struct{})}
		m.opening[name] = call
		m.mu.Unlock()

		h, err := m.openHandle(name, path)

		m.mu.Lock()
		if err == nil && m.shutdown.Load() {
			err = fmt.Errorf("namespace manager is shut down")
		}
		if err == nil {
			h.lifecycle.RLock()
			m.handles[name] = h
			m.retained.Add(1)
			h.element = m.order.PushBack(h)
		}
		call.err = err
		delete(m.opening, name)
		close(call.done)
		m.mu.Unlock()

		if err != nil {
			if h != nil {
				_ = m.closeHandle(name, h)
			}
			return nil, err
		}
		m.collectExcess(name)
		return h, nil
	}
}

func (m *Manager) openHandle(name, path string) (*handle, error) {
	if m.cfg.BeforeOpen != nil {
		if err := m.cfg.BeforeOpen(name, path); err != nil {
			return nil, fmt.Errorf("prepare namespace open: %w", err)
		}
	}

	h := &handle{name: name, path: path, writeSlot: make(chan struct{}, 1)}
	h.touch()
	return h, nil
}

func (m *Manager) readPool(h *handle) (*sql.DB, error) {
	h.poolsMu.Lock()
	defer h.poolsMu.Unlock()
	if h.readDB != nil {
		return h.readDB, nil
	}

	dsn := h.path + "?_pragma=query_only(1)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open namespace read pool: %w", err)
	}
	db.SetMaxOpenConns(m.cfg.ReadConns)
	db.SetMaxIdleConns(1)
	if m.cfg.OnReadOpen != nil {
		if err := m.cfg.OnReadOpen(h.name, h.path, db); err != nil {
			if m.cfg.OnReadClose != nil {
				m.cfg.OnReadClose(h.name, h.path, db)
			}
			_ = db.Close()
			return nil, fmt.Errorf("configure namespace read pool: %w", err)
		}
	}
	h.readDB = db
	return db, nil
}

func (m *Manager) writePool(h *handle) (*sql.DB, error) {
	h.poolsMu.Lock()
	defer h.poolsMu.Unlock()
	if h.writeDB != nil {
		return h.writeDB, nil
	}

	dsn := h.path + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open namespace write pool: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if m.cfg.OnWriteOpen != nil {
		if err := m.cfg.OnWriteOpen(h.name, h.path, db); err != nil {
			if m.cfg.OnWriteClose != nil {
				m.cfg.OnWriteClose(h.name, h.path, db)
			}
			_ = db.Close()
			return nil, fmt.Errorf("configure namespace write pool: %w", err)
		}
	}
	h.writeDB = db
	return db, nil
}

// WithRead executes fn under the pinned namespace read lock.
func (m *Manager) WithRead(name, path string, fn func(*sql.DB) error) error {
	h, err := m.getOrOpenPinned(name, path)
	if err != nil {
		return err
	}
	defer func() {
		h.lifecycle.RUnlock()
		m.trimExcess()
	}()
	readDB, err := m.readPool(h)
	if err != nil {
		return err
	}
	return fn(readDB)
}

// WithWrite executes fn under the pinned namespace write lock.
func (m *Manager) WithWrite(name, path string, fn func(*sql.DB) error) error {
	return m.WithWriteContext(context.Background(), name, path, fn)
}

// WithWriteContext bounds the wait for a namespace writer by ctx.
func (m *Manager) WithWriteContext(ctx context.Context, name, path string, fn func(*sql.DB) error) error {
	h, err := m.getOrOpenPinned(name, path)
	if err != nil {
		return err
	}
	defer func() {
		h.lifecycle.RUnlock()
		m.trimExcess()
	}()

	waitStarted := time.Now()
	select {
	case h.writeSlot <- struct{}{}:
		defer func() { <-h.writeSlot }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.cfg.OnWriteWait != nil {
		m.cfg.OnWriteWait(name, time.Since(waitStarted))
	}
	h.touch()
	writeDB, err := m.writePool(h)
	if err != nil {
		return err
	}
	return fn(writeDB)
}

// CloseHandle closes and removes a namespace handle from the manager.
func (m *Manager) CloseHandle(name string) error {
	m.mu.Lock()
	h, ok := m.handles[name]
	if ok {
		delete(m.handles, name)
		m.retained.Add(-1)
		if h.element != nil {
			m.order.Remove(h.element)
			h.element = nil
		}
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	h.lifecycle.Lock()
	defer h.lifecycle.Unlock()
	return m.closeHandle(name, h)
}

func (h *handle) touch() {
	atomic.StoreInt64(&h.lastUsed, time.Now().UnixNano())
	h.referenced.Store(true)
}

func (h *handle) lastUsedAt() time.Time {
	n := atomic.LoadInt64(&h.lastUsed)
	if n == 0 {
		return time.Time{}
	}
	return time.Unix(0, n)
}

func (h *handle) closePools() error {
	var errs []error
	if h.readDB != nil {
		errs = append(errs, h.readDB.Close())
	}
	if h.writeDB != nil {
		errs = append(errs, h.writeDB.Close())
	}
	return errors.Join(errs...)
}

func (m *Manager) closeHandle(name string, h *handle) error {
	h.poolsMu.Lock()
	defer h.poolsMu.Unlock()
	if h.readDB != nil && m.cfg.OnReadClose != nil {
		m.cfg.OnReadClose(name, h.path, h.readDB)
	}
	if h.writeDB != nil && m.cfg.OnWriteClose != nil {
		m.cfg.OnWriteClose(name, h.path, h.writeDB)
	}
	return h.closePools()
}

func (m *Manager) collectExcess(exclude string) {
	for {
		m.mu.Lock()
		if len(m.handles) <= m.cfg.MaxRetained {
			m.mu.Unlock()
			return
		}

		var victim *handle
		for pass := range 2 {
			scans := min(m.order.Len(), 64)
			for range scans {
				element := m.order.Front()
				if element == nil {
					break
				}
				h := element.Value.(*handle)
				m.order.MoveToBack(element)
				if h.name == exclude {
					continue
				}
				referenced := h.referenced.Swap(false)
				if pass == 0 && referenced {
					continue
				}
				if !h.lifecycle.TryLock() {
					continue
				}
				victim = h
				break
			}
			if victim != nil {
				break
			}
		}
		if victim == nil {
			m.mu.Unlock()
			return
		}
		delete(m.handles, victim.name)
		m.retained.Add(-1)
		if victim.element != nil {
			m.order.Remove(victim.element)
			victim.element = nil
		}
		m.mu.Unlock()

		m.closeHandle(victim.name, victim)
		victim.lifecycle.Unlock()
	}
}

func (m *Manager) trimExcess() {
	if m.retained.Load() > int64(m.cfg.MaxRetained) {
		m.collectExcess("")
	}
}

// OpenCount returns the number of retained namespace handles.
func (m *Manager) OpenCount() int {
	return int(m.retained.Load())
}
