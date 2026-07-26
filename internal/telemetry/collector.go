package telemetry

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type bucketState struct {
	bucket  Bucket
	dirty   bool
	version uint64
}

type bucketKey struct {
	namespace string
	start     int64
}

// Collector owns in-memory telemetry buckets and their persistence lifecycle.
type Collector struct {
	store    *store
	mu       sync.Mutex
	buckets  map[bucketKey]*bucketState
	now      func() time.Time
	stop     chan struct{}
	done     chan struct{}
	close    sync.Once
	interval time.Duration
}

// Open creates the telemetry collector in dataDir.
func Open(dataDir string) (*Collector, error) {
	store, err := openStore(dataDir)
	if err != nil {
		return nil, err
	}
	collector := newCollector(store, 5*time.Second, time.Now)
	go collector.run()
	return collector, nil
}

func newCollector(store *store, interval time.Duration, now func() time.Time) *Collector {
	return &Collector{
		store:    store,
		buckets:  make(map[bucketKey]*bucketState),
		now:      now,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		interval: interval,
	}
}

// Record adds one request to the current in-memory bucket.
func (c *Collector) Record(event Event) {
	if event.Namespace == "" {
		return
	}
	now := c.now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()

	state := c.state(event.Namespace, now)
	if event.Write {
		state.bucket.Writes++
		if !event.Streaming {
			state.bucket.WriteDuration.observe(event.Duration)
		}
	} else {
		state.bucket.Reads++
		if !event.Streaming {
			state.bucket.ReadDuration.observe(event.Duration)
		}
	}
	if event.Status >= 400 {
		state.bucket.Errors++
	}
	if event.QuotaRejected {
		state.bucket.QuotaRejected++
	}
	if event.Busy {
		state.bucket.BusyErrors++
	}
	if event.Timeout {
		state.bucket.Timeouts++
	}
	if event.ResponseBytes > 0 {
		state.bucket.ResponseBytes += uint64(event.ResponseBytes)
	}
	operationName := event.Operation
	if operationName == "" {
		operationName = "other"
	}
	if _, ok := state.bucket.Operations[operationName]; !ok && len(state.bucket.Operations) >= maxOps {
		operationName = "other"
	}
	operation := state.bucket.Operations[operationName]
	operation.Count++
	if event.Status >= 400 {
		operation.Errors++
	}
	if !event.Streaming {
		operation.Duration.observe(event.Duration)
	}
	state.bucket.Operations[operationName] = operation
	state.bucket.LastActivityAt = now.Unix()
	state.dirty = true
	state.version++
}

// RecordWriteWait records time spent waiting for the namespace writer.
func (c *Collector) RecordWriteWait(namespace string, duration time.Duration) {
	if namespace == "" {
		return
	}
	now := c.now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state(namespace, now)
	state.bucket.WriteWait.observe(duration)
	state.dirty = true
	state.version++
}

// RecordStorage updates the current bucket's latest storage sample.
func (c *Collector) RecordStorage(namespace string, storage StorageSnapshot) {
	if namespace == "" {
		return
	}
	now := c.now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.state(namespace, now)
	copy := storage
	state.bucket.Storage = &copy
	state.dirty = true
	state.version++
}

func (c *Collector) state(namespace string, now time.Time) *bucketState {
	start := now.Truncate(fiveMinutes)
	key := bucketKey{namespace: namespace, start: start.Unix()}
	state, ok := c.buckets[key]
	if !ok {
		state = &bucketState{bucket: newBucket(namespace, start, fiveMinutes)}
		c.buckets[key] = state
	}
	return state
}

// Buckets returns persisted and current telemetry for a namespace window.
func (c *Collector) Buckets(namespace string, from, to time.Time) ([]Bucket, error) {
	persisted, err := c.store.query(namespace, from, to)
	if err != nil {
		return nil, err
	}
	byKey := make(map[bucketKey]Bucket, len(persisted))
	for _, bucket := range persisted {
		byKey[bucketKey{namespace: namespace, start: bucket.BucketStart}] = bucket
	}
	c.mu.Lock()
	for key, state := range c.buckets {
		if key.namespace != namespace || key.start < from.Unix() || key.start >= to.Unix() {
			continue
		}
		byKey[key] = cloneBucket(state.bucket)
	}
	c.mu.Unlock()
	out := make([]Bucket, 0, len(byKey))
	for _, bucket := range byKey {
		out = append(out, bucket)
	}
	return out, nil
}

// Latest returns the latest persisted bucket for each namespace.
func (c *Collector) Latest() (map[string]Bucket, error) {
	latest, err := c.store.latest()
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	for key, state := range c.buckets {
		current, ok := latest[key.namespace]
		if !ok || state.bucket.BucketStart >= current.BucketStart {
			latest[key.namespace] = cloneBucket(state.bucket)
		}
	}
	c.mu.Unlock()
	return latest, nil
}

// Purge deletes all telemetry for a namespace.
func (c *Collector) Purge(namespace string) error {
	c.mu.Lock()
	for key := range c.buckets {
		if key.namespace == namespace {
			delete(c.buckets, key)
		}
	}
	c.mu.Unlock()
	return c.store.purge(namespace)
}

// Flush persists only dirty in-memory buckets.
func (c *Collector) Flush() error {
	type snapshot struct {
		key     bucketKey
		version uint64
		bucket  Bucket
	}
	c.mu.Lock()
	snapshots := make([]snapshot, 0)
	for key, state := range c.buckets {
		if !state.dirty {
			continue
		}
		copy := cloneBucket(state.bucket)
		snapshots = append(snapshots, snapshot{key: key, version: state.version, bucket: copy})
		state.dirty = false
	}
	c.mu.Unlock()

	buckets := make([]Bucket, 0, len(snapshots))
	for _, snapshot := range snapshots {
		buckets = append(buckets, snapshot.bucket)
	}
	if err := c.store.upsert(buckets); err != nil {
		c.mu.Lock()
		for _, snapshot := range snapshots {
			if state := c.buckets[snapshot.key]; state != nil {
				state.dirty = true
			}
		}
		c.mu.Unlock()
		return err
	}

	currentStart := c.now().UTC().Truncate(fiveMinutes).Unix()
	c.mu.Lock()
	for _, snapshot := range snapshots {
		state := c.buckets[snapshot.key]
		if state != nil && !state.dirty && state.version == snapshot.version && snapshot.key.start < currentStart {
			delete(c.buckets, snapshot.key)
		}
	}
	c.mu.Unlock()
	return nil
}

func cloneBucket(bucket Bucket) Bucket {
	clone := bucket
	clone.ReadDuration.Counts = append([]uint64(nil), bucket.ReadDuration.Counts...)
	clone.WriteDuration.Counts = append([]uint64(nil), bucket.WriteDuration.Counts...)
	clone.WriteWait.Counts = append([]uint64(nil), bucket.WriteWait.Counts...)
	clone.Operations = make(map[string]OperationAggregate, len(bucket.Operations))
	for name, operation := range bucket.Operations {
		operation.Duration.Counts = append([]uint64(nil), operation.Duration.Counts...)
		clone.Operations[name] = operation
	}
	if bucket.Storage != nil {
		storage := *bucket.Storage
		clone.Storage = &storage
	}
	return clone
}

func (c *Collector) run() {
	defer close(c.done)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	lastMaintenance := time.Time{}
	for {
		select {
		case <-ticker.C:
			_ = c.Flush()
			now := c.now().UTC()
			if lastMaintenance.IsZero() || !now.Truncate(time.Hour).Equal(lastMaintenance) {
				if err := c.store.maintain(now); err == nil {
					lastMaintenance = now.Truncate(time.Hour)
				}
			}
		case <-c.stop:
			return
		}
	}
}

// Close flushes pending telemetry and closes telemetry.db.
func (c *Collector) Close() error {
	var closeErr error
	c.close.Do(func() {
		close(c.stop)
		<-c.done
		closeErr = errors.Join(c.Flush(), c.store.maintain(c.now().UTC()), c.store.close())
	})
	if closeErr != nil {
		return fmt.Errorf("close telemetry: %w", closeErr)
	}
	return nil
}
