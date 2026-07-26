package telemetry

import "time"

const (
	fiveMinutes = 5 * time.Minute
	oneHour     = time.Hour
	maxOps      = 32
)

// Event describes one completed namespace request.
type Event struct {
	Namespace     string
	Operation     string
	Write         bool
	Status        int
	Duration      time.Duration
	ResponseBytes int64
	QuotaRejected bool
	Busy          bool
	Timeout       bool
	Streaming     bool
}

// StorageSnapshot is the latest inexpensive SQLite storage sample.
type StorageSnapshot struct {
	UsedBytes      int64 `json:"used_bytes"`
	AllocatedBytes int64 `json:"allocated_bytes"`
	WALBytes       int64 `json:"wal_bytes"`
	DiskBytes      int64 `json:"disk_bytes"`
}

// OperationAggregate contains bounded per-operation telemetry.
type OperationAggregate struct {
	Count    uint64    `json:"count"`
	Errors   uint64    `json:"errors"`
	Duration Histogram `json:"duration"`
}

// Bucket is one persisted namespace telemetry bucket.
type Bucket struct {
	Namespace      string                        `json:"namespace"`
	BucketStart    int64                         `json:"bucket_start"`
	Resolution     int64                         `json:"resolution_seconds"`
	Reads          uint64                        `json:"reads"`
	Writes         uint64                        `json:"writes"`
	Errors         uint64                        `json:"errors"`
	QuotaRejected  uint64                        `json:"quota_rejected"`
	BusyErrors     uint64                        `json:"busy_errors"`
	Timeouts       uint64                        `json:"timeouts"`
	ResponseBytes  uint64                        `json:"response_bytes"`
	ReadDuration   Histogram                     `json:"read_duration"`
	WriteDuration  Histogram                     `json:"write_duration"`
	WriteWait      Histogram                     `json:"write_wait"`
	Operations     map[string]OperationAggregate `json:"operations"`
	Storage        *StorageSnapshot              `json:"storage,omitempty"`
	LastActivityAt int64                         `json:"last_activity_at"`
}

func newBucket(namespace string, start time.Time, resolution time.Duration) Bucket {
	return Bucket{
		Namespace:     namespace,
		BucketStart:   start.Unix(),
		Resolution:    int64(resolution / time.Second),
		ReadDuration:  newHistogram(),
		WriteDuration: newHistogram(),
		WriteWait:     newHistogram(),
		Operations:    make(map[string]OperationAggregate),
	}
}

// Merge combines another bucket into b.
func (b *Bucket) Merge(other Bucket) {
	b.Reads += other.Reads
	b.Writes += other.Writes
	b.Errors += other.Errors
	b.QuotaRejected += other.QuotaRejected
	b.BusyErrors += other.BusyErrors
	b.Timeouts += other.Timeouts
	b.ResponseBytes += other.ResponseBytes
	b.ReadDuration.merge(other.ReadDuration)
	b.WriteDuration.merge(other.WriteDuration)
	b.WriteWait.merge(other.WriteWait)
	if b.Operations == nil {
		b.Operations = make(map[string]OperationAggregate)
	}
	for name, operation := range other.Operations {
		current := b.Operations[name]
		current.Count += operation.Count
		current.Errors += operation.Errors
		current.Duration.merge(operation.Duration)
		b.Operations[name] = current
	}
	if other.Storage != nil {
		copy := *other.Storage
		b.Storage = &copy
	}
	if other.LastActivityAt > b.LastActivityAt {
		b.LastActivityAt = other.LastActivityAt
	}
}
