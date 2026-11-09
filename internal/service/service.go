package service

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/k2b-dev/rsql/internal/domain"
	"github.com/k2b-dev/rsql/internal/namespace"
	"github.com/k2b-dev/rsql/internal/sse"
	"github.com/k2b-dev/rsql/internal/store/control"
	"github.com/k2b-dev/rsql/internal/store/sqlite"
	"github.com/k2b-dev/rsql/internal/telemetry"
)

var namespaceNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

const (
	DefaultNamespacePageSize = 100
	MaxNamespacePageSize     = 500
)

// NamespacePage is one bounded page of namespace control-plane records.
type NamespacePage struct {
	Data       []map[string]any `json:"data"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// Service orchestrates all rsql operations.
type Service struct {
	dataDir   string
	registry  *control.Registry
	ns        *namespace.Manager
	broker    *sse.Broker
	telemetry *telemetry.Collector
}

// New creates a service instance.
func New(dataDir string, idleTimeout time.Duration, maxOpenNamespaces, namespaceReadConns int) (*Service, error) {
	reg, err := control.Open(dataDir)
	if err != nil {
		return nil, err
	}
	collector, err := telemetry.Open(dataDir)
	if err != nil {
		_ = reg.Close()
		return nil, err
	}
	s := &Service{
		dataDir:   dataDir,
		registry:  reg,
		telemetry: collector,
		broker:    sse.NewBroker(),
	}
	s.ns = namespace.NewManager(namespace.Config{
		IdleTimeout: idleTimeout,
		MaxRetained: maxOpenNamespaces,
		ReadConns:   namespaceReadConns,
		BeforeOpen:  s.beforeNamespaceOpen,
		OnReadOpen: func(_ string, path string, db *sql.DB) error {
			sqlite.RegisterNamespacePool(path, db)
			return nil
		},
		OnWriteOpen: func(name, path string, db *sql.DB) error {
			sqlite.RegisterNamespacePool(path, db)
			return configureNamespaceWrite(name, db)
		},
		OnReadClose: func(_, _ string, db *sql.DB) {
			sqlite.UnregisterNamespacePool(db)
		},
		OnWriteClose: func(_, _ string, db *sql.DB) {
			sqlite.UnregisterNamespacePool(db)
		},
		OnWriteWait: collector.RecordWriteWait,
	})
	return s, nil
}

func defaultNamespaceConfig() domain.NamespaceConfig {
	return domain.NamespaceConfig{
		JournalMode:  "wal",
		Synchronous:  "full",
		BusyTimeout:  5000,
		QueryTimeout: 10000,
		ForeignKeys:  domain.BoolPtr(true),
	}
}

func normalizeNamespaceConfig(cfg domain.NamespaceConfig) (domain.NamespaceConfig, error) {
	cfg.Synchronous = strings.ToLower(cfg.Synchronous)
	if cfg.Synchronous == "" {
		cfg.Synchronous = "full"
	}
	if cfg.Synchronous != "full" && cfg.Synchronous != "normal" {
		return domain.NamespaceConfig{}, domain.NewError(
			domain.ErrInvalidRequest,
			400,
			"synchronous must be full or normal",
		)
	}
	return cfg, nil
}

func configureNamespaceWrite(_ string, db *sql.DB) error {
	if err := sqlite.EnsureInternalSchema(db); err != nil {
		return err
	}
	cfg := defaultNamespaceConfig()
	ok, err := sqlite.GetMeta(db, "namespace_config", "self", &cfg)
	if err != nil {
		return err
	}
	if !ok {
		if err := sqlite.PutMeta(db, "namespace_config", "self", cfg); err != nil {
			return err
		}
	}
	return sqlite.ApplyNamespaceConfig(db, cfg)
}

// Close closes all service resources.
func (s *Service) Close() error {
	var errs []error
	if s.ns != nil {
		errs = append(errs, s.ns.Close())
	}
	if s.telemetry != nil {
		errs = append(errs, s.telemetry.Close())
	}
	if s.registry != nil {
		errs = append(errs, s.registry.Close())
	}
	return errors.Join(errs...)
}

func (s *Service) beforeNamespaceOpen(name, path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.NewError(
				domain.ErrNamespaceNotFound,
				404,
				fmt.Sprintf("Namespace '%s' does not exist", name),
			)
		}
		return fmt.Errorf("stat namespace database: %w", err)
	}
	return nil
}

func validateNamespaceName(name string) error {
	if !namespaceNameRe.MatchString(name) {
		return fmt.Errorf("invalid namespace name %q", name)
	}
	return nil
}

// CreateNamespace creates a namespace DB and persists config.
func (s *Service) CreateNamespace(req domain.NamespaceDefinition) (map[string]any, error) {
	if err := validateNamespaceName(req.Name); err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}
	var err error
	req.Config, err = normalizeNamespaceConfig(req.Config)
	if err != nil {
		return nil, err
	}
	path := control.PathFor(s.dataDir, req.Name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to create namespace directory", err)
	}
	if err := s.registry.Create(req.Name, path); err != nil {
		if errors.Is(err, control.ErrAlreadyExists) {
			return nil, domain.WrapError(domain.ErrConflict, 409, "namespace already exists", err)
		}
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to create namespace record", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		_ = s.registry.Delete(req.Name)
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to create namespace database", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		_ = s.registry.Delete(req.Name)
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to close namespace database", err)
	}

	err = s.ns.WithWrite(req.Name, path, func(db *sql.DB) error {
		if err := sqlite.EnsureInternalSchema(db); err != nil {
			return err
		}
		if err := sqlite.PutMeta(db, "namespace_config", "self", req.Config); err != nil {
			return err
		}
		if err := sqlite.ApplyNamespaceConfig(db, req.Config); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// Init failed — roll back the registry entry, the open DB handle,
		// and the on-disk file so a retried create starts from clean state.
		_ = s.ns.CloseHandle(req.Name)
		_ = os.Remove(path)
		_ = s.registry.Delete(req.Name)
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to initialize namespace", err)
	}
	_ = s.telemetry.Purge(req.Name)

	return map[string]any{"name": req.Name, "config": req.Config}, nil
}

// ListNamespaces lists one page of active namespaces.
func (s *Service) ListNamespaces(limit int, cursor string) (NamespacePage, error) {
	if limit == 0 {
		limit = DefaultNamespacePageSize
	}
	if limit < 1 || limit > MaxNamespacePageSize {
		return NamespacePage{}, domain.NewError(
			domain.ErrInvalidRequest,
			400,
			fmt.Sprintf("limit must be between 1 and %d", MaxNamespacePageSize),
		)
	}
	if cursor != "" {
		if err := validateNamespaceName(cursor); err != nil {
			return NamespacePage{}, domain.NewError(domain.ErrInvalidRequest, 400, "invalid namespace cursor")
		}
	}
	recs, nextCursor, err := s.registry.ListPage(limit, cursor)
	if err != nil {
		return NamespacePage{}, domain.WrapError(domain.ErrInternal, 500, "failed to list namespaces", err)
	}
	names := make([]string, len(recs))
	for index, record := range recs {
		names[index] = record.Name
	}
	latest, err := s.telemetry.LatestFor(names)
	if err != nil {
		return NamespacePage{}, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace activity", err)
	}
	out := make([]map[string]any, 0, len(recs))
	for _, r := range recs {
		item := map[string]any{
			"name":       r.Name,
			"created_at": r.CreatedAt,
		}
		if bucket, ok := latest[r.Name]; ok {
			if bucket.LastActivityAt > 0 {
				item["last_activity_at"] = time.Unix(bucket.LastActivityAt, 0).UTC().Format(time.RFC3339)
			}
			if bucket.Storage != nil {
				item["storage"] = bucket.Storage
			}
		}
		out = append(out, item)
	}
	return NamespacePage{Data: out, NextCursor: nextCursor}, nil
}

// GetNamespace returns namespace info.
func (s *Service) GetNamespace(name string) (map[string]any, error) {
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}
	cfg, err := s.GetNamespaceConfig(name)
	if err != nil {
		return nil, err
	}
	return map[string]any{"name": rec.Name, "db_path": rec.DBPath, "created_at": rec.CreatedAt, "config": cfg}, nil
}

// UpdateNamespaceConfig updates and applies namespace config.
func (s *Service) UpdateNamespaceConfig(name string, cfg domain.NamespaceConfig) (map[string]any, error) {
	var err error
	cfg, err = normalizeNamespaceConfig(cfg)
	if err != nil {
		return nil, err
	}
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}

	err = s.ns.WithWrite(name, rec.DBPath, func(db *sql.DB) error {
		// Administrative configuration must remain writable while persisting the
		// new read_only setting. Failed updates discard this pool below.
		if _, err := db.Exec("PRAGMA query_only=0"); err != nil {
			return err
		}
		if err := sqlite.EnsureInternalSchema(db); err != nil {
			return err
		}
		writable := cfg
		writable.ReadOnly = false
		if err := sqlite.ApplyNamespaceConfig(db, writable); err != nil {
			return err
		}
		if err := sqlite.PutMeta(db, "namespace_config", "self", cfg); err != nil {
			return err
		}
		if cfg.ReadOnly {
			_, err := db.Exec("PRAGMA query_only=1")
			return err
		}
		return nil
	})
	if err != nil {
		_ = s.ns.CloseHandle(name)
		return nil, mapErr(err)
	}

	return map[string]any{"name": name, "config": cfg}, nil
}

// DeleteNamespace deletes namespace DB and lookup record.
func (s *Service) DeleteNamespace(name string) error {
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}

	if err := s.ns.CloseHandle(name); err != nil {
		return domain.WrapError(domain.ErrInternal, 500, "failed to close namespace handle", err)
	}
	if err := os.Remove(rec.DBPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return domain.WrapError(domain.ErrInternal, 500, "failed to remove namespace database", err)
	}
	if err := s.registry.Delete(name); err != nil {
		if err != sql.ErrNoRows {
			return domain.WrapError(domain.ErrInternal, 500, "failed to delete namespace", err)
		}
	}
	_ = s.telemetry.Purge(name)
	return nil
}

// DuplicateNamespace duplicates namespace DB into a new namespace.
func (s *Service) DuplicateNamespace(source, target string) (map[string]any, error) {
	if err := validateNamespaceName(target); err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}
	src, err := s.registry.Get(source)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", source))
		}
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to load source namespace", err)
	}
	targetPath := control.PathFor(s.dataDir, target)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to create target namespace directory", err)
	}
	if err := s.registry.Create(target, targetPath); err != nil {
		return nil, domain.WrapError(domain.ErrConflict, 409, "target namespace already exists", err)
	}

	if err := s.ns.WithWrite(source, src.DBPath, func(db *sql.DB) error {
		_, err := db.Exec(`VACUUM INTO ?`, targetPath)
		return err
	}); err != nil {
		_ = s.registry.Delete(target)
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to duplicate namespace", err)
	}
	_ = s.telemetry.Purge(target)
	return map[string]any{"source": source, "target": target}, nil
}

// ExportNamespace returns namespace DB path for file download.
func (s *Service) ExportNamespace(name string) (string, error) {
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return "", domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}
	exportDir := filepath.Join(s.dataDir, "exports")
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return "", domain.WrapError(domain.ErrInternal, 500, "failed to create export directory", err)
	}
	exportPath := filepath.Join(exportDir, fmt.Sprintf("%s-%d.db", name, time.Now().UTC().UnixNano()))
	_ = os.Remove(exportPath)

	err = s.ns.WithWrite(name, rec.DBPath, func(db *sql.DB) error {
		_, err := db.Exec(`VACUUM INTO ?`, exportPath)
		return err
	})
	if err != nil {
		// VACUUM may have left a partial file; remove it so failed exports
		// do not accumulate on disk.
		_ = os.Remove(exportPath)
		return "", domain.WrapError(domain.ErrInternal, 500, "failed to export namespace snapshot", err)
	}
	return exportPath, nil
}

// ImportNamespaceDB replaces namespace DB with uploaded database content.
func (s *Service) ImportNamespaceDB(name string, src io.Reader) error {
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}
	cfg, err := s.GetNamespaceConfig(name)
	if err != nil {
		return err
	}

	tmp := rec.DBPath + ".import.tmp"
	backup := rec.DBPath + ".import.backup"
	_ = os.Remove(tmp)
	_ = os.Remove(backup)
	defer os.Remove(tmp)
	defer os.Remove(backup)

	f, err := os.Create(tmp)
	if err != nil {
		return domain.WrapError(domain.ErrInternal, 500, "failed to create import temp file", err)
	}
	reader := src
	if cfg.MaxDBSize > 0 {
		reader = io.LimitReader(src, cfg.MaxDBSize+1)
	}
	written, copyErr := io.Copy(f, reader)
	if copyErr != nil {
		_ = f.Close()
		return domain.WrapError(domain.ErrInternal, 500, "failed to write import temp file", copyErr)
	}
	if err := f.Close(); err != nil {
		return domain.WrapError(domain.ErrInternal, 500, "failed to close import temp file", err)
	}
	if cfg.MaxDBSize > 0 && written > cfg.MaxDBSize {
		return domain.NewError(domain.ErrQuotaExceeded, 507, "import exceeds namespace storage quota")
	}
	if err := validateImportDatabase(tmp, cfg); err != nil {
		return mapErr(err)
	}

	if err := s.ns.CloseHandle(name); err != nil {
		return domain.WrapError(domain.ErrInternal, 500, "failed to close namespace handle", err)
	}
	if err := os.Rename(rec.DBPath, backup); err != nil {
		return domain.WrapError(domain.ErrInternal, 500, "failed to preserve current namespace database", err)
	}
	if err := os.Rename(tmp, rec.DBPath); err != nil {
		_ = os.Rename(backup, rec.DBPath)
		return domain.WrapError(domain.ErrInternal, 500, "failed to replace namespace database", err)
	}
	if err := s.ns.WithWrite(name, rec.DBPath, func(*sql.DB) error { return nil }); err != nil {
		_ = s.ns.CloseHandle(name)
		_ = os.Remove(rec.DBPath)
		_ = os.Rename(backup, rec.DBPath)
		return domain.WrapError(domain.ErrInternal, 500, "failed to configure imported namespace database", err)
	}
	_ = os.Remove(backup)
	return nil
}

func validateImportDatabase(path string, cfg domain.NamespaceConfig) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open imported database: %w", err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()

	if _, err := db.Exec("PRAGMA journal_mode = DELETE"); err != nil {
		return fmt.Errorf("prepare imported database: %w", err)
	}
	if err := sqlite.EnsureInternalSchema(db); err != nil {
		return err
	}
	if err := sqlite.PutMeta(db, "namespace_config", "self", cfg); err != nil {
		return err
	}
	var check string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil {
		return fmt.Errorf("check imported database: %w", err)
	}
	if check != "ok" {
		return fmt.Errorf("invalid imported database: %s", check)
	}
	storage, err := sqlite.Storage(db, path, cfg.MaxDBSize)
	if err != nil {
		return err
	}
	if cfg.MaxDBSize > 0 && storage.AllocatedBytes > cfg.MaxDBSize {
		return domain.NewError(domain.ErrQuotaExceeded, 507, "import exceeds namespace storage quota")
	}
	return nil
}

// ImportNamespaceCSV imports CSV rows into a table.
func (s *Service) ImportNamespaceCSV(name, table string, src io.Reader, meta json.RawMessage) (map[string]any, error) {
	rec, err := s.registry.Get(name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.NewError(domain.ErrNamespaceNotFound, 404, fmt.Sprintf("Namespace '%s' does not exist", name))
		}
		return nil, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}

	reader := csv.NewReader(src)
	headers, err := reader.Read()
	if err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, 400, "invalid csv header")
	}

	rows := make([]map[string]any, 0, 128)
	for {
		recRow, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, domain.NewError(domain.ErrInvalidRequest, 400, "invalid csv row")
		}
		row := make(map[string]any, len(headers))
		for i, h := range headers {
			if i < len(recRow) {
				row[h] = recRow[i]
			}
		}
		rows = append(rows, row)
	}

	var inserted int
	var ids []any
	includeIDs := s.broker.Count(name) > 0
	err = s.ns.WithWrite(name, rec.DBPath, func(db *sql.DB) error {
		if err := sqlite.EnsureInternalSchema(db); err != nil {
			return err
		}
		result, err := sqlite.InsertRowsWithOptions(db, table, rows, sqlite.InsertOptions{
			IncludeIDs: includeIDs,
		})
		if err != nil {
			return err
		}
		inserted = result.Affected
		if len(result.IDs) > 0 {
			ids = append([]any(nil), result.IDs...)
		}
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	if len(ids) > 0 {
		s.publishBulk(name, table, "bulk_insert", ids, meta)
	}

	return map[string]any{"inserted": inserted}, nil
}

// TablesList lists tables/views in namespace.
func (s *Service) TablesList(ns string) ([]map[string]any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		rows, err := sqlite.ListTables(db)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

func (s *Service) pathForNamespace(name string) (string, error) {
	if err := validateNamespaceName(name); err != nil {
		return "", domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}
	return control.PathFor(s.dataDir, name), nil
}

func (s *Service) GetNamespaceConfig(name string) (domain.NamespaceConfig, error) {
	path, err := s.pathForNamespace(name)
	if err != nil {
		return domain.NamespaceConfig{}, err
	}

	cfg := defaultNamespaceConfig()
	err = s.ns.WithRead(name, path, func(db *sql.DB) error {
		ok, err := sqlite.GetMeta(db, "namespace_config", "self", &cfg)
		if err != nil {
			return err
		}
		if !ok {
			cfg = defaultNamespaceConfig()
		}
		return nil
	})
	if err != nil {
		return domain.NamespaceConfig{}, mapErr(err)
	}
	return cfg, nil
}

// CreateTableOrView creates a table/view and emits schema event.
func (s *Service) CreateTableOrView(ns string, req domain.TableCreateRequest) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := sqlite.EnsureInternalSchema(db); err != nil {
			return err
		}
		if err := sqlite.CreateTableOrView(db, req); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return mapErr(err)
	}
	s.publishSchema(ns, req.Name, map[string]any{"type": "create", "request": req}, req.Meta)
	return nil
}

// GetTable returns table/view schema details.
func (s *Service) GetTable(ns, name string) (map[string]any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		row, err := sqlite.GetTable(db, name)
		if err != nil {
			return err
		}
		out = row
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// UpdateTableOrView updates table/view schema and emits schema event.
func (s *Service) UpdateTableOrView(ns, name string, req domain.TableUpdateRequest) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		return sqlite.UpdateTableOrView(db, name, req)
	})
	if err != nil {
		return mapErr(err)
	}
	target := name
	if req.Rename != "" {
		target = req.Rename
	}
	s.publishSchema(ns, target, map[string]any{"type": "update", "request": req}, req.Meta)
	return nil
}

// DeleteTableOrView deletes table/view and emits schema event.
func (s *Service) DeleteTableOrView(ns, name string, meta json.RawMessage) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		return sqlite.DeleteTableOrView(db, name, meta)
	})
	if err != nil {
		return mapErr(err)
	}
	s.publishSchema(ns, name, map[string]any{"type": "delete"}, meta)
	return nil
}

// CreateIndex creates an index.
func (s *Service) CreateIndex(ns, table string, req domain.IndexCreateRequest) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		return sqlite.CreateIndex(db, table, req)
	})
	if err != nil {
		return mapErr(err)
	}
	s.publishSchema(ns, table, map[string]any{"type": "create_index", "request": req}, req.Meta)
	return nil
}

// DeleteIndex deletes an index.
func (s *Service) DeleteIndex(ns, table, idx string, meta json.RawMessage) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		return sqlite.DeleteIndex(db, table, idx, meta)
	})
	if err != nil {
		return mapErr(err)
	}
	s.publishSchema(ns, table, map[string]any{"type": "delete_index", "index": idx}, meta)
	return nil
}

// ListRows lists rows.
func (s *Service) ListRows(ns, table string, query map[string][]string) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out any
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		rows, err := sqlite.ListRows(db, table, query)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// AssertTableOrViewExists fails with a typed namespace/not-found error when
// the namespace or the table/view does not exist. Used by streaming
// endpoints to pre-flight existence before sending HTTP headers.
func (s *Service) AssertTableOrViewExists(ns, table string) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		if !sqlite.TableOrViewExists(db, table) {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// ExportTableCSV streams a table's rows as CSV-encoded bytes to w.
//
// The query map mirrors the shape used by ListRows so HTTP callers can pass
// r.URL.Query() unchanged. Reads run under the namespace read lock; this
// function does not publish SSE or changelog events because read paths are
// not observable in the existing rsql model.
func (s *Service) ExportTableCSV(ns, table string, query map[string][]string, w io.Writer) error {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return err
	}
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		return sqlite.StreamCSV(db, table, query, w)
	})
	if err != nil {
		return mapErr(err)
	}
	return nil
}

// GetRow fetches a row by id.
func (s *Service) GetRow(ns, table string, id any) (map[string]any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		row, err := sqlite.GetRowByID(db, table, id)
		if err != nil {
			return err
		}
		out = row
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// assertWritableTable returns a typed read-only error if the named object
// is a view. Used at the top of every row-write callback so the rejection
// message stays consistent and the action verb is the only thing that
// varies per call site.
func assertWritableTable(db *sql.DB, table, action string) error {
	isView, err := sqlite.IsView(db, table)
	if err != nil {
		return err
	}
	if isView {
		return domain.NewError(domain.ErrReadOnly, 405, fmt.Sprintf("Cannot %s view '%s'. Views are read-only.", action, table))
	}
	return nil
}

// InsertRows inserts one or many rows.
func (s *Service) InsertRows(ns, table string, rows []map[string]any, prefer string, meta json.RawMessage) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	hasSubscribers := s.broker.Count(ns) > 0
	var out []map[string]any
	var ids []any
	var inserted int
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := assertWritableTable(db, table, "insert into"); err != nil {
			return err
		}
		result, err := sqlite.InsertRowsWithOptions(db, table, rows, sqlite.InsertOptions{
			Resolution:  prefer,
			IncludeRows: prefer == "return=representation" || (hasSubscribers && len(rows) == 1),
			IncludeIDs:  hasSubscribers && len(rows) > 1,
		})
		if err != nil {
			return err
		}
		out = result.Rows
		ids = result.IDs
		inserted = result.Affected
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}

	if len(rows) > 1 {
		s.publishBulk(ns, table, "bulk_insert", ids, meta)
	} else if len(out) > 0 {
		s.publishRow(ns, table, "insert", out[0], meta)
	}

	if prefer == "return=representation" {
		return map[string]any{"data": out}, nil
	}
	return map[string]any{"inserted": inserted}, nil
}

// UpdateRow updates one row.
func (s *Service) UpdateRow(ns, table string, id any, payload map[string]any, prefer string, meta json.RawMessage) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	hasSubscribers := s.broker.Count(ns) > 0
	var rows []map[string]any
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := assertWritableTable(db, table, "update"); err != nil {
			return err
		}
		result, err := sqlite.UpdateRowsWithOptions(
			db,
			table,
			payload,
			`id = ?`,
			[]any{id},
			sqlite.MutationOptions{
				IncludeRows: prefer == "return=representation" || hasSubscribers,
			},
		)
		if err != nil {
			return err
		}
		if result.Affected == 0 {
			return sql.ErrNoRows
		}
		rows = result.Rows
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}

	if len(rows) > 0 {
		s.publishRow(ns, table, "update", rows[0], meta)
	}
	if prefer == "return=representation" {
		return map[string]any{"data": rows}, nil
	}
	return map[string]any{"updated": 1}, nil
}

// DeleteRow deletes one row.
func (s *Service) DeleteRow(ns, table string, id any, prefer string, meta json.RawMessage) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	hasSubscribers := s.broker.Count(ns) > 0
	var rows []map[string]any
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := assertWritableTable(db, table, "delete from"); err != nil {
			return err
		}
		result, err := sqlite.DeleteRowsWithOptions(
			db,
			table,
			`id = ?`,
			[]any{id},
			sqlite.MutationOptions{
				IncludeRows: prefer == "return=representation" || hasSubscribers,
			},
		)
		if err != nil {
			return err
		}
		if result.Affected == 0 {
			return sql.ErrNoRows
		}
		rows = result.Rows
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}

	if len(rows) > 0 {
		s.publishRow(ns, table, "delete", rows[0], meta)
	}
	if prefer == "return=representation" {
		return map[string]any{"data": rows}, nil
	}
	return map[string]any{"deleted": 1}, nil
}

// BulkUpdate updates rows by filter.
func (s *Service) BulkUpdate(ns, table string, query map[string][]string, payload map[string]any, prefer string, meta json.RawMessage) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}

	where, args, err := sqlite.BuildWhereForBulk(query)
	if err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}

	var ids []any
	var rows []map[string]any
	var updated int
	hasSubscribers := s.broker.Count(ns) > 0
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := assertWritableTable(db, table, "update"); err != nil {
			return err
		}
		result, err := sqlite.UpdateRowsWithOptions(
			db,
			table,
			payload,
			where,
			args,
			sqlite.MutationOptions{
				IncludeRows: prefer == "return=representation",
				IncludeIDs:  hasSubscribers,
			},
		)
		if err != nil {
			return err
		}
		ids = result.IDs
		rows = result.Rows
		updated = result.Affected
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	if len(ids) > 0 {
		s.publishBulk(ns, table, "bulk_update", ids, meta)
	}
	if prefer == "return=representation" {
		return map[string]any{"data": rows}, nil
	}
	return map[string]any{"updated": updated}, nil
}

// BulkDelete deletes rows by filter.
func (s *Service) BulkDelete(ns, table string, query map[string][]string, prefer string, meta json.RawMessage) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}

	where, args, err := sqlite.BuildWhereForBulk(query)
	if err != nil {
		return nil, domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}

	var ids []any
	var rows []map[string]any
	var deleted int
	hasSubscribers := s.broker.Count(ns) > 0
	err = s.ns.WithWrite(ns, path, func(db *sql.DB) error {
		if err := assertWritableTable(db, table, "delete from"); err != nil {
			return err
		}
		result, err := sqlite.DeleteRowsWithOptions(
			db,
			table,
			where,
			args,
			sqlite.MutationOptions{
				IncludeRows: prefer == "return=representation",
				IncludeIDs:  hasSubscribers,
			},
		)
		if err != nil {
			return err
		}
		ids = result.IDs
		rows = result.Rows
		deleted = result.Affected
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	if len(ids) > 0 {
		s.publishBulk(ns, table, "bulk_delete", ids, meta)
	}
	if prefer == "return=representation" {
		return map[string]any{"data": rows}, nil
	}
	return map[string]any{"deleted": deleted}, nil
}

// Query executes read-only SQL statements.
func (s *Service) Query(ns string, req sqlite.QueryRequest) (any, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out any
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		result, err := sqlite.ExecuteReadOnly(db, req)
		if err != nil {
			return err
		}
		out = result
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "sql_not_read_only") {
			return nil, domain.NewError(domain.ErrSQLNotReadOnly, 400, "Only read-only SELECT queries are allowed")
		}
		return nil, mapErr(err)
	}
	return out, nil
}

// Changelog returns schema changelog entries.
func (s *Service) Changelog(ns, table string, limit, offset int) ([]sqlite.ChangelogEntry, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return nil, err
	}
	var out []sqlite.ChangelogEntry
	err = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		entries, err := sqlite.ListChangelog(db, table, limit, offset)
		if err != nil {
			return err
		}
		out = entries
		return nil
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return out, nil
}

// Subscribe registers SSE subscriptions.
func (s *Service) Subscribe(ns string, tables []string) (uint64, <-chan domain.SSEEvent, error) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return 0, nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil, domain.NewError(
				domain.ErrNamespaceNotFound,
				404,
				fmt.Sprintf("Namespace '%s' does not exist", ns),
			)
		}
		return 0, nil, domain.WrapError(domain.ErrInternal, 500, "failed to stat namespace", err)
	}
	id, ch := s.broker.Subscribe(ns, tables)
	return id, ch, nil
}

// Unsubscribe removes SSE subscription.
func (s *Service) Unsubscribe(ns string, id uint64) {
	s.broker.Unsubscribe(ns, id)
}

func (s *Service) publishRow(ns, table, action string, row map[string]any, meta json.RawMessage) {
	if s.broker.Count(ns) == 0 {
		return
	}
	ev := domain.SSEEvent{
		Namespace: ns,
		Table:     table,
		Action:    action,
		Row:       row,
		Meta:      meta,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	s.broker.Publish(ns, ev)
	s.publishViewInvalidations(ns, table, action)
}

func (s *Service) publishBulk(ns, table, action string, ids []any, meta json.RawMessage) {
	if s.broker.Count(ns) == 0 {
		return
	}
	ev := domain.SSEEvent{
		Namespace: ns,
		Table:     table,
		Action:    action,
		RowCount:  len(ids),
		RowIDs:    ids,
		Meta:      meta,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	s.broker.Publish(ns, ev)
	s.publishViewInvalidations(ns, table, action)
}

func (s *Service) publishSchema(ns, table string, detail map[string]any, meta json.RawMessage) {
	if s.broker.Count(ns) == 0 {
		return
	}
	ev := domain.SSEEvent{
		Namespace: ns,
		Table:     table,
		Action:    "schema_change",
		Detail:    detail,
		Meta:      meta,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	s.broker.Publish(ns, ev)
}

func (s *Service) publishViewInvalidations(ns, sourceTable, sourceAction string) {
	path, err := s.pathForNamespace(ns)
	if err != nil {
		return
	}
	_ = s.ns.WithRead(ns, path, func(db *sql.DB) error {
		views, err := sqlite.ListViewSources(db)
		if err != nil {
			return err
		}
		for _, view := range views {
			for _, st := range view.Tables {
				if st == sourceTable {
					s.broker.Publish(ns, domain.SSEEvent{
						Namespace:    ns,
						Table:        view.Name,
						Action:       "source_changed",
						SourceTable:  sourceTable,
						SourceAction: sourceAction,
						Timestamp:    time.Now().UTC().Format(time.RFC3339),
					})
					break
				}
			}
		}
		return nil
	})
}

func mapErr(err error) error {
	// Fast paths: typed errors flow through unchanged, sql.ErrNoRows
	// becomes a 404, and the validation sentinel from sqlite/errors.go is
	// the canonical 400 source.
	var derr *domain.Error
	if errors.As(err, &derr) {
		return derr
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewError(domain.ErrNotFound, 404, "resource not found")
	}
	if errors.Is(err, sqlite.ErrValidation) {
		return domain.NewError(domain.ErrValidationFailed, 400, err.Error())
	}
	if errors.Is(err, sqlite.ErrQuotaTooSmall) {
		return domain.NewError(domain.ErrInvalidRequest, 400, err.Error())
	}
	if sqlite.IsFullError(err) {
		return domain.NewError(domain.ErrQuotaExceeded, 507, "namespace storage quota exceeded")
	}
	if sqlite.IsBusyError(err) {
		return domain.NewError(domain.ErrDatabaseBusy, 503, "namespace database is busy")
	}

	// Remaining string-based mapping: identifier / structural validation
	// errors carry "invalid …" messages, and SQLite driver constraint
	// violations carry "constraint …". Those messages are stable across
	// our codebase (and the modernc.org/sqlite driver) so a substring
	// match is safe enough.
	msg := err.Error()
	if strings.Contains(msg, "invalid") {
		return domain.NewError(domain.ErrInvalidRequest, 400, msg)
	}
	if strings.Contains(strings.ToLower(msg), "constraint") {
		return domain.NewError(domain.ErrConflict, 409, msg)
	}
	return domain.WrapError(domain.ErrInternal, 500, "internal server error", err)
}

// BuildImportPath returns absolute import path target for namespace utility commands.
func (s *Service) BuildImportPath(name string) (string, error) {
	if err := validateNamespaceName(name); err != nil {
		return "", err
	}
	path := control.PathFor(s.dataDir, name)
	return filepath.Abs(path)
}

// WithNamespaceRead runs a raw read callback (used by handlers for advanced needs).
func (s *Service) WithNamespaceRead(name string, fn func(*sql.DB) error) error {
	path, err := s.pathForNamespace(name)
	if err != nil {
		return err
	}
	if err := s.ns.WithRead(name, path, fn); err != nil {
		return mapErr(err)
	}
	return nil
}

// WithNamespaceWrite runs a raw write callback (used by handlers for advanced needs).
func (s *Service) WithNamespaceWrite(name string, fn func(*sql.DB) error) error {
	path, err := s.pathForNamespace(name)
	if err != nil {
		return err
	}
	if err := s.ns.WithWrite(name, path, fn); err != nil {
		return mapErr(err)
	}
	return nil
}

// NormalizeMeta extracts the optional audit-meta passthrough from a parsed
// request body. The wire key is `_meta` so it cannot collide with a
// user-defined column named `meta`.
func NormalizeMeta(payload map[string]json.RawMessage) json.RawMessage {
	if raw, ok := payload["_meta"]; ok {
		return raw
	}
	return nil
}

// ParseRowsPayload normalizes POST row payloads.
func ParseRowsPayload(body map[string]json.RawMessage) ([]map[string]any, json.RawMessage, error) {
	meta := NormalizeMeta(body)

	if rawRows, ok := body["rows"]; ok {
		var rows []map[string]any
		if err := json.Unmarshal(rawRows, &rows); err != nil {
			return nil, nil, fmt.Errorf("invalid rows payload")
		}
		return rows, meta, nil
	}

	row := make(map[string]any)
	for k, raw := range body {
		if k == "_meta" {
			continue
		}
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, nil, fmt.Errorf("invalid field %s", k)
		}
		row[k] = v
	}
	if len(row) == 0 {
		return nil, nil, fmt.Errorf("empty row payload")
	}
	return []map[string]any{row}, meta, nil
}

// ParseJSONMap decodes raw body into map.
func ParseJSONMap(raw []byte) (map[string]json.RawMessage, error) {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, fmt.Errorf("invalid json body")
	}
	if body == nil {
		body = map[string]json.RawMessage{}
	}
	return body, nil
}

// ParseUpdatePayload decodes update payload and extracts meta.
func ParseUpdatePayload(body map[string]json.RawMessage) (map[string]any, json.RawMessage, error) {
	meta := NormalizeMeta(body)
	out := make(map[string]any)
	for k, raw := range body {
		if k == "_meta" {
			continue
		}
		var v any
		dec := json.NewDecoder(strings.NewReader(string(raw)))
		dec.UseNumber()
		if err := dec.Decode(&v); err != nil {
			return nil, nil, fmt.Errorf("invalid field %s", k)
		}
		out[k] = normalizeJSONNumbers(v)
	}
	if len(out) == 0 {
		return nil, nil, fmt.Errorf("empty update payload")
	}
	return out, meta, nil
}

func normalizeJSONNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(t))
		for k, v2 := range t {
			m[k] = normalizeJSONNumbers(v2)
		}
		return m
	case []any:
		arr := make([]any, len(t))
		for i := range t {
			arr[i] = normalizeJSONNumbers(t[i])
		}
		return arr
	case json.Number:
		if strings.Contains(t.String(), ".") {
			f, err := t.Float64()
			if err == nil {
				return f
			}
			return t.String()
		}
		i, err := t.Int64()
		if err == nil {
			return i
		}
		f, err := t.Float64()
		if err == nil {
			return f
		}
		return t.String()
	default:
		return v
	}
}

// ParseID converts path id to integer id.
func ParseID(idStr string) (int64, error) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid row id")
	}
	return id, nil
}
