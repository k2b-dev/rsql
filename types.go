package rsql

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
)

var (
	namespacePattern  = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
	identifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

// APIError is a structured non-2xx rsql response.
type APIError struct {
	Status  int
	Code    string
	Message string
	Header  http.Header
}

func (e *APIError) Error() string {
	return fmt.Sprintf("rsql API error %d %s: %s", e.Status, e.Code, e.Message)
}

// StreamResponse is a successful streaming response. Callers must close Body.
type StreamResponse struct {
	Status int
	Header http.Header
	Body   io.ReadCloser
}

// NamespaceConfig configures one namespace database.
type NamespaceConfig struct {
	JournalMode  string `json:"journal_mode"`
	Synchronous  string `json:"synchronous"`
	BusyTimeout  int    `json:"busy_timeout"`
	MaxDBSize    int64  `json:"max_db_size,omitempty"`
	QueryTimeout int    `json:"query_timeout"`
	ForeignKeys  *bool  `json:"foreign_keys,omitempty"`
	ReadOnly     bool   `json:"read_only,omitempty"`
}

// DefaultNamespaceConfig returns the server's standard namespace settings.
func DefaultNamespaceConfig() NamespaceConfig {
	foreignKeys := true
	return NamespaceConfig{
		JournalMode:  "wal",
		Synchronous:  "full",
		BusyTimeout:  5000,
		QueryTimeout: 10000,
		ForeignKeys:  &foreignKeys,
	}
}

// NamespaceDefinition describes a namespace to create.
type NamespaceDefinition struct {
	Name   string           `json:"name"`
	Config *NamespaceConfig `json:"config,omitempty"`
}

// NamespaceRecord is namespace control-plane metadata.
type NamespaceRecord struct {
	Name           string           `json:"name"`
	CreatedAt      string           `json:"created_at,omitempty"`
	DBPath         string           `json:"db_path,omitempty"`
	Config         *NamespaceConfig `json:"config,omitempty"`
	LastActivityAt string           `json:"last_activity_at,omitempty"`
	Storage        *StorageSnapshot `json:"storage,omitempty"`
}

// NamespaceListOptions selects one namespace page.
type NamespaceListOptions struct {
	Limit  int
	Cursor string
}

// NamespacePage is one bounded page of namespace records.
type NamespacePage struct {
	Data       []NamespaceRecord `json:"data"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// ColumnDefinition describes one table column.
type ColumnDefinition struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	NotNull    bool            `json:"not_null,omitempty"`
	Unique     bool            `json:"unique,omitempty"`
	Default    any             `json:"default,omitempty"`
	Index      bool            `json:"index,omitempty"`
	Pattern    string          `json:"pattern,omitempty"`
	MaxLength  int             `json:"max_length,omitempty"`
	Min        *float64        `json:"min,omitempty"`
	Max        *float64        `json:"max,omitempty"`
	Auto       bool            `json:"auto,omitempty"`
	Options    []string        `json:"options,omitempty"`
	Formula    string          `json:"formula,omitempty"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	PrimaryKey bool            `json:"primary_key,omitempty"`
	ReadOnly   bool            `json:"read_only,omitempty"`
}

// TableCreateRequest creates a table or view.
type TableCreateRequest struct {
	Type     string             `json:"type"`
	Name     string             `json:"name"`
	Metadata json.RawMessage    `json:"metadata,omitempty"`
	Columns  []ColumnDefinition `json:"columns,omitempty"`
	SQL      string             `json:"sql,omitempty"`
	Meta     json.RawMessage    `json:"_meta,omitempty"`
}

// TableUpdateRequest updates a table or view.
type TableUpdateRequest struct {
	Rename        string             `json:"rename,omitempty"`
	AddColumns    []ColumnDefinition `json:"add_columns,omitempty"`
	DropColumns   []string           `json:"drop_columns,omitempty"`
	RenameColumns map[string]string  `json:"rename_columns,omitempty"`
	SQL           string             `json:"sql,omitempty"`
	Metadata      json.RawMessage    `json:"metadata,omitempty"`
	Meta          json.RawMessage    `json:"_meta,omitempty"`
}

// IndexCreateRequest creates a regular, unique, or FTS index.
type IndexCreateRequest struct {
	Type    string          `json:"type"`
	Name    string          `json:"name,omitempty"`
	Columns []string        `json:"columns"`
	Meta    json.RawMessage `json:"_meta,omitempty"`
}

// QueryStatement is one read-only SQL statement.
type QueryStatement struct {
	SQL    string `json:"sql"`
	Params []any  `json:"params,omitempty"`
}

// QueryRequest executes a single statement or a batch.
type QueryRequest struct {
	SQL        string           `json:"sql,omitempty"`
	Params     []any            `json:"params,omitempty"`
	Statements []QueryStatement `json:"statements,omitempty"`
}

// ListMeta describes a paginated row result.
type ListMeta struct {
	TotalCount  *int `json:"total_count,omitempty"`
	FilterCount *int `json:"filter_count,omitempty"`
	Limit       int  `json:"limit"`
	Offset      int  `json:"offset"`
}

// RowsResponse represents regular and aggregate row-list responses.
type RowsResponse struct {
	Data []map[string]any `json:"data"`
	Meta *ListMeta        `json:"meta,omitempty"`
}

// MutationResult represents row mutation response variants.
type MutationResult struct {
	Data     []map[string]any `json:"data,omitempty"`
	Inserted int              `json:"inserted,omitempty"`
	Updated  int              `json:"updated,omitempty"`
	Deleted  int              `json:"deleted,omitempty"`
}

// MutateOptions controls mutation response and audit metadata.
type MutateOptions struct {
	Prefer string
	Meta   json.RawMessage
}

const (
	PreferReturnRepresentation = "return=representation"
	PreferMergeDuplicates      = "resolution=merge-duplicates"
	PreferIgnoreDuplicates     = "resolution=ignore-duplicates"
)

// ChangelogEntry is one schema changelog item.
type ChangelogEntry struct {
	ID        int             `json:"id"`
	Timestamp string          `json:"timestamp"`
	Action    string          `json:"action"`
	Table     string          `json:"table"`
	Detail    json.RawMessage `json:"detail"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

// OverviewWindow identifies a supported telemetry window.
type OverviewWindow string

const (
	Window1Hour  OverviewWindow = "1h"
	Window24Hour OverviewWindow = "24h"
	Window7Days  OverviewWindow = "7d"
	Window30Days OverviewWindow = "30d"
)

// StorageSnapshot contains lightweight storage values.
type StorageSnapshot struct {
	UsedBytes      int64 `json:"used_bytes"`
	AllocatedBytes int64 `json:"allocated_bytes"`
	WALBytes       int64 `json:"wal_bytes"`
	DiskBytes      int64 `json:"disk_bytes"`
}

// OverviewLatency summarizes request latency.
type OverviewLatency struct {
	Count     uint64  `json:"count"`
	AverageMS float64 `json:"average_ms"`
	P50MS     float64 `json:"p50_ms"`
	P95MS     float64 `json:"p95_ms"`
	P99MS     float64 `json:"p99_ms"`
}

// NamespaceOverview is the tenant-facing operational summary.
type NamespaceOverview struct {
	GeneratedAt string `json:"generated_at"`
	Window      struct {
		Name       OverviewWindow `json:"name"`
		From       string         `json:"from"`
		To         string         `json:"to"`
		Resolution string         `json:"resolution"`
	} `json:"window"`
	Health struct {
		Status string   `json:"status"`
		Mode   string   `json:"mode"`
		Issues []string `json:"issues"`
	} `json:"health"`
	Storage struct {
		StorageSnapshot
		ReclaimableBytes  int64    `json:"reclaimable_bytes"`
		QuotaBytes        int64    `json:"quota_bytes"`
		RemainingBytes    *int64   `json:"remaining_bytes"`
		UsageRatio        *float64 `json:"usage_ratio"`
		DatabaseFileBytes int64    `json:"database_file_bytes"`
		SHMBytes          int64    `json:"shm_bytes"`
	} `json:"storage"`
	Activity struct {
		Requests       uint64  `json:"requests"`
		Reads          uint64  `json:"reads"`
		Writes         uint64  `json:"writes"`
		Errors         uint64  `json:"errors"`
		ErrorRate      float64 `json:"error_rate"`
		ResponseBytes  uint64  `json:"response_bytes"`
		LastActivityAt *string `json:"last_activity_at"`
		Series         []struct {
			Timestamp string `json:"timestamp"`
			Reads     uint64 `json:"reads"`
			Writes    uint64 `json:"writes"`
			Errors    uint64 `json:"errors"`
		} `json:"series"`
	} `json:"activity"`
	ReadLatency     OverviewLatency `json:"read_latency"`
	WriteLatency    OverviewLatency `json:"write_latency"`
	WriteContention struct {
		Wait            OverviewLatency `json:"wait"`
		BusyErrors      uint64          `json:"busy_errors"`
		Timeouts        uint64          `json:"timeouts"`
		QuotaRejections uint64          `json:"quota_rejections"`
	} `json:"write_contention"`
	Schema struct {
		Tables     int `json:"tables"`
		Views      int `json:"views"`
		Indexes    int `json:"indexes"`
		FTSIndexes int `json:"fts_indexes"`
	} `json:"schema"`
	Realtime struct {
		Subscribers int `json:"subscribers"`
	} `json:"realtime"`
	TopOperations []struct {
		Name      string          `json:"name"`
		Count     uint64          `json:"count"`
		Errors    uint64          `json:"errors"`
		ErrorRate float64         `json:"error_rate"`
		Latency   OverviewLatency `json:"latency"`
	} `json:"top_operations"`
	Growth *struct {
		BytesPerDay     float64 `json:"bytes_per_day"`
		SampleCount     int     `json:"sample_count"`
		ProjectedFullAt *string `json:"projected_full_at"`
	} `json:"growth,omitempty"`
}

// SSEEvent represents one namespace change event.
type SSEEvent struct {
	Namespace    string          `json:"namespace,omitempty"`
	Table        string          `json:"table"`
	Action       string          `json:"action"`
	SourceTable  string          `json:"source_table,omitempty"`
	SourceAction string          `json:"source_action,omitempty"`
	Row          map[string]any  `json:"row,omitempty"`
	RowCount     int             `json:"row_count,omitempty"`
	RowIDs       []any           `json:"row_ids,omitempty"`
	Detail       map[string]any  `json:"detail,omitempty"`
	Meta         json.RawMessage `json:"_meta,omitempty"`
	Timestamp    string          `json:"timestamp"`
}

// SubscribeOptions filters a namespace event stream.
type SubscribeOptions struct {
	Tables []string
}

// TableExportOptions configures streamed CSV export.
type TableExportOptions struct {
	Query map[string][]string
	BOM   bool
}

// ImportOptions describes a streamed multipart upload.
type ImportOptions struct {
	Filename    string
	ContentType string
	Body        io.Reader
}

func validateNamespace(namespace string) error {
	if !namespacePattern.MatchString(namespace) {
		return fmt.Errorf("invalid namespace %q", namespace)
	}
	return nil
}

func validateIdentifier(kind, identifier string) error {
	if !identifierPattern.MatchString(identifier) {
		return fmt.Errorf("invalid %s %q", kind, identifier)
	}
	return nil
}
