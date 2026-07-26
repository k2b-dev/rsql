package telemetry

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCollectorPersistsAndAggregatesBuckets(t *testing.T) {
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 10, 2, 0, 0, time.UTC)
	collector := newCollector(store, time.Hour, func() time.Time { return now })

	collector.Record(Event{
		Namespace:     "tenant-a",
		Operation:     "rows.list",
		Duration:      3 * time.Millisecond,
		Status:        200,
		ResponseBytes: 120,
	})
	collector.Record(Event{
		Namespace:     "tenant-a",
		Operation:     "rows.insert",
		Write:         true,
		Duration:      8 * time.Millisecond,
		Status:        507,
		QuotaRejected: true,
	})
	collector.RecordWriteWait("tenant-a", 2*time.Millisecond)
	collector.RecordStorage("tenant-a", StorageSnapshot{UsedBytes: 100, AllocatedBytes: 120})

	if err := collector.Flush(); err != nil {
		t.Fatal(err)
	}
	buckets, err := store.query("tenant-a", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 {
		t.Fatalf("buckets = %d, want 1", len(buckets))
	}
	bucket := buckets[0]
	if bucket.Reads != 1 || bucket.Writes != 1 || bucket.Errors != 1 || bucket.QuotaRejected != 1 {
		t.Fatalf("unexpected counters: %#v", bucket)
	}
	if bucket.Storage == nil || bucket.Storage.UsedBytes != 100 {
		t.Fatalf("unexpected storage: %#v", bucket.Storage)
	}
	if got := bucket.Operations["rows.list"].Count; got != 1 {
		t.Fatalf("rows.list count = %d", got)
	}
	if err := store.close(); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorConcurrentRecordDoesNotLoseEvents(t *testing.T) {
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	collector := newCollector(store, time.Hour, func() time.Time { return now })

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				collector.Record(Event{Namespace: "tenant", Operation: "rows.get", Status: 200})
			}
		}()
	}
	wg.Wait()
	if err := collector.Flush(); err != nil {
		t.Fatal(err)
	}
	buckets, err := store.query("tenant", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 || buckets[0].Reads != 1000 {
		t.Fatalf("unexpected bucket: %#v", buckets)
	}
	_ = store.close()
}

func TestStoreRollsUpAndPurgesNamespace(t *testing.T) {
	dir := t.TempDir()
	store, err := openStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	first := newBucket("tenant", now.Add(-2*time.Hour), fiveMinutes)
	first.Reads = 2
	second := newBucket("tenant", now.Add(-2*time.Hour+fiveMinutes), fiveMinutes)
	second.Writes = 3
	if err := store.upsert([]Bucket{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := store.maintain(now); err != nil {
		t.Fatal(err)
	}
	hourly, err := store.queryResolution("tenant", now.Add(-3*time.Hour), now, int64(oneHour/time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(hourly) != 1 || hourly[0].Reads != 2 || hourly[0].Writes != 3 {
		t.Fatalf("unexpected rollup: %#v", hourly)
	}
	if err := store.purge("tenant"); err != nil {
		t.Fatal(err)
	}
	latest, err := store.latest()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := latest["tenant"]; ok {
		t.Fatal("tenant telemetry still present after purge")
	}
	if got := filepath.Base(filepath.Join(dir, "telemetry.db")); got != "telemetry.db" {
		t.Fatalf("unexpected telemetry path: %s", got)
	}
	_ = store.close()
}

func TestCollectorPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	first.Record(Event{Namespace: "tenant", Operation: "rows.list", Status: 200})
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = second.Close() })
	now := time.Now().UTC()
	buckets, err := second.Buckets("tenant", now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(buckets) != 1 || buckets[0].Reads != 1 {
		t.Fatalf("unexpected persisted telemetry: %#v", buckets)
	}
}

func BenchmarkCollectorRecord(b *testing.B) {
	now := time.Now().UTC()
	collector := newCollector(nil, time.Hour, func() time.Time { return now })
	event := Event{
		Namespace:     "tenant",
		Operation:     "rows.list",
		Status:        200,
		Duration:      time.Millisecond,
		ResponseBytes: 256,
	}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			collector.Record(event)
		}
	})
}
