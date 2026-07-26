package domain

// NamespaceOverview is the tenant-facing operational summary for one database.
type NamespaceOverview struct {
	GeneratedAt     string                  `json:"generated_at"`
	Window          OverviewWindow          `json:"window"`
	Health          OverviewHealth          `json:"health"`
	Storage         OverviewStorage         `json:"storage"`
	Activity        OverviewActivity        `json:"activity"`
	ReadLatency     OverviewLatency         `json:"read_latency"`
	WriteLatency    OverviewLatency         `json:"write_latency"`
	WriteContention OverviewWriteContention `json:"write_contention"`
	Schema          OverviewSchema          `json:"schema"`
	Realtime        OverviewRealtime        `json:"realtime"`
	TopOperations   []OverviewOperation     `json:"top_operations"`
	Growth          *OverviewGrowth         `json:"growth,omitempty"`
}

// OverviewWindow describes the selected aggregation period.
type OverviewWindow struct {
	Name       string `json:"name"`
	From       string `json:"from"`
	To         string `json:"to"`
	Resolution string `json:"resolution"`
}

// OverviewHealth describes the current namespace mode and detected issues.
type OverviewHealth struct {
	Status string   `json:"status"`
	Mode   string   `json:"mode"`
	Issues []string `json:"issues"`
}

// OverviewStorage describes live SQLite storage and the configured quota.
type OverviewStorage struct {
	UsedBytes         int64    `json:"used_bytes"`
	AllocatedBytes    int64    `json:"allocated_bytes"`
	ReclaimableBytes  int64    `json:"reclaimable_bytes"`
	QuotaBytes        int64    `json:"quota_bytes"`
	RemainingBytes    *int64   `json:"remaining_bytes"`
	UsageRatio        *float64 `json:"usage_ratio"`
	DatabaseFileBytes int64    `json:"database_file_bytes"`
	WALBytes          int64    `json:"wal_bytes"`
	SHMBytes          int64    `json:"shm_bytes"`
	DiskBytes         int64    `json:"disk_bytes"`
}

// OverviewActivity contains traffic totals and its time series.
type OverviewActivity struct {
	Requests       uint64                  `json:"requests"`
	Reads          uint64                  `json:"reads"`
	Writes         uint64                  `json:"writes"`
	Errors         uint64                  `json:"errors"`
	ErrorRate      float64                 `json:"error_rate"`
	ResponseBytes  uint64                  `json:"response_bytes"`
	LastActivityAt *string                 `json:"last_activity_at"`
	Series         []OverviewActivityPoint `json:"series"`
}

// OverviewActivityPoint is one retained traffic bucket.
type OverviewActivityPoint struct {
	Timestamp string `json:"timestamp"`
	Reads     uint64 `json:"reads"`
	Writes    uint64 `json:"writes"`
	Errors    uint64 `json:"errors"`
}

// OverviewLatency summarizes a fixed mergeable histogram.
type OverviewLatency struct {
	Count     uint64  `json:"count"`
	AverageMS float64 `json:"average_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
}

// OverviewWriteContention summarizes namespace writer queueing.
type OverviewWriteContention struct {
	Wait         OverviewLatency `json:"wait"`
	BusyErrors   uint64          `json:"busy_errors"`
	Timeouts     uint64          `json:"timeouts"`
	QuotaRejects uint64          `json:"quota_rejections"`
}

// OverviewSchema contains inexpensive schema object counts.
type OverviewSchema struct {
	Tables     int `json:"tables"`
	Views      int `json:"views"`
	Indexes    int `json:"indexes"`
	FTSIndexes int `json:"fts_indexes"`
}

// OverviewRealtime contains live event-stream state.
type OverviewRealtime struct {
	Subscribers int `json:"subscribers"`
}

// OverviewOperation summarizes one stable API operation.
type OverviewOperation struct {
	Name      string          `json:"name"`
	Count     uint64          `json:"count"`
	Errors    uint64          `json:"errors"`
	ErrorRate float64         `json:"error_rate"`
	Latency   OverviewLatency `json:"latency"`
}

// OverviewGrowth estimates recent allocated-storage growth.
type OverviewGrowth struct {
	BytesPerDay     float64 `json:"bytes_per_day"`
	SampleCount     int     `json:"sample_count"`
	ProjectedFullAt *string `json:"projected_full_at"`
}
