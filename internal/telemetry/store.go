package telemetry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type store struct {
	db *sql.DB
}

func openStore(dataDir string) (*store, error) {
	path := filepath.Join(dataDir, "telemetry.db")
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open telemetry database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS telemetry_buckets (
			namespace TEXT NOT NULL,
			resolution_seconds INTEGER NOT NULL,
			bucket_start INTEGER NOT NULL,
			payload_json TEXT NOT NULL,
			PRIMARY KEY (namespace, resolution_seconds, bucket_start)
		);
		CREATE INDEX IF NOT EXISTS telemetry_buckets_time
			ON telemetry_buckets (resolution_seconds, bucket_start);
		CREATE TABLE IF NOT EXISTS telemetry_latest (
			namespace TEXT PRIMARY KEY,
			payload_json TEXT NOT NULL
		);
	`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize telemetry database: %w", err)
	}
	return &store{db: db}, nil
}

func (s *store) close() error {
	return s.db.Close()
}

func (s *store) upsert(buckets []Bucket) error {
	if len(buckets) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin telemetry flush: %w", err)
	}
	defer tx.Rollback()

	for _, bucket := range buckets {
		payload, err := json.Marshal(bucket)
		if err != nil {
			return fmt.Errorf("encode telemetry bucket: %w", err)
		}
		if _, err := tx.Exec(`
			INSERT INTO telemetry_buckets(namespace, resolution_seconds, bucket_start, payload_json)
			VALUES(?, ?, ?, ?)
			ON CONFLICT(namespace, resolution_seconds, bucket_start)
			DO UPDATE SET payload_json = excluded.payload_json
		`, bucket.Namespace, bucket.Resolution, bucket.BucketStart, string(payload)); err != nil {
			return fmt.Errorf("upsert telemetry bucket: %w", err)
		}
		if bucket.Resolution == int64(fiveMinutes/time.Second) {
			if _, err := tx.Exec(`
				INSERT INTO telemetry_latest(namespace, payload_json) VALUES(?, ?)
				ON CONFLICT(namespace) DO UPDATE SET payload_json = excluded.payload_json
			`, bucket.Namespace, string(payload)); err != nil {
				return fmt.Errorf("upsert latest telemetry: %w", err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit telemetry flush: %w", err)
	}
	return nil
}

func (s *store) query(namespace string, from, to time.Time) ([]Bucket, error) {
	resolution := int64(fiveMinutes / time.Second)
	if to.Sub(from) > 48*time.Hour {
		resolution = int64(oneHour / time.Second)
	}
	rows, err := s.db.Query(`
		SELECT payload_json FROM telemetry_buckets
		WHERE namespace = ? AND resolution_seconds = ?
			AND bucket_start >= ? AND bucket_start < ?
		ORDER BY bucket_start
	`, namespace, resolution, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("query telemetry buckets: %w", err)
	}
	defer rows.Close()

	var out []Bucket
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan telemetry bucket: %w", err)
		}
		var bucket Bucket
		if err := json.Unmarshal([]byte(payload), &bucket); err != nil {
			return nil, fmt.Errorf("decode telemetry bucket: %w", err)
		}
		out = append(out, bucket)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate telemetry buckets: %w", err)
	}

	// The current incomplete hour has not been rolled up yet.
	if resolution == int64(oneHour/time.Second) {
		currentHour := to.Truncate(time.Hour)
		current, err := s.queryResolution(namespace, currentHour, to, int64(fiveMinutes/time.Second))
		if err != nil {
			return nil, err
		}
		if len(current) > 0 {
			aggregate := newBucket(namespace, currentHour, oneHour)
			for _, bucket := range current {
				aggregate.Merge(bucket)
			}
			out = append(out, aggregate)
		}
	}
	return out, nil
}

func (s *store) queryResolution(namespace string, from, to time.Time, resolution int64) ([]Bucket, error) {
	rows, err := s.db.Query(`
		SELECT payload_json FROM telemetry_buckets
		WHERE namespace = ? AND resolution_seconds = ?
			AND bucket_start >= ? AND bucket_start < ?
		ORDER BY bucket_start
	`, namespace, resolution, from.Unix(), to.Unix())
	if err != nil {
		return nil, fmt.Errorf("query telemetry resolution: %w", err)
	}
	defer rows.Close()

	var out []Bucket
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, fmt.Errorf("scan telemetry resolution: %w", err)
		}
		var bucket Bucket
		if err := json.Unmarshal([]byte(payload), &bucket); err != nil {
			return nil, fmt.Errorf("decode telemetry resolution: %w", err)
		}
		out = append(out, bucket)
	}
	return out, rows.Err()
}

func (s *store) latest() (map[string]Bucket, error) {
	rows, err := s.db.Query(`SELECT namespace, payload_json FROM telemetry_latest`)
	if err != nil {
		return nil, fmt.Errorf("query latest telemetry: %w", err)
	}
	defer rows.Close()

	out := make(map[string]Bucket)
	for rows.Next() {
		var namespace, payload string
		if err := rows.Scan(&namespace, &payload); err != nil {
			return nil, fmt.Errorf("scan latest telemetry: %w", err)
		}
		var bucket Bucket
		if err := json.Unmarshal([]byte(payload), &bucket); err != nil {
			return nil, fmt.Errorf("decode latest telemetry: %w", err)
		}
		out[namespace] = bucket
	}
	return out, rows.Err()
}

func (s *store) latestFor(namespaces []string) (map[string]Bucket, error) {
	if len(namespaces) == 0 {
		return map[string]Bucket{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(namespaces)), ",")
	args := make([]any, len(namespaces))
	for index, namespace := range namespaces {
		args[index] = namespace
	}
	rows, err := s.db.Query(
		`SELECT namespace, payload_json FROM telemetry_latest WHERE namespace IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query latest telemetry page: %w", err)
	}
	defer rows.Close()

	out := make(map[string]Bucket, len(namespaces))
	for rows.Next() {
		var namespace, payload string
		if err := rows.Scan(&namespace, &payload); err != nil {
			return nil, fmt.Errorf("scan latest telemetry page: %w", err)
		}
		var bucket Bucket
		if err := json.Unmarshal([]byte(payload), &bucket); err != nil {
			return nil, fmt.Errorf("decode latest telemetry page: %w", err)
		}
		out[namespace] = bucket
	}
	return out, rows.Err()
}

func (s *store) purge(namespace string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin telemetry purge: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM telemetry_buckets WHERE namespace = ?`, namespace); err != nil {
		return fmt.Errorf("purge telemetry buckets: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM telemetry_latest WHERE namespace = ?`, namespace); err != nil {
		return fmt.Errorf("purge latest telemetry: %w", err)
	}
	return tx.Commit()
}

func (s *store) maintain(now time.Time) error {
	currentHour := now.Truncate(time.Hour).Unix()
	rows, err := s.db.Query(`
		SELECT payload_json FROM telemetry_buckets
		WHERE resolution_seconds = ? AND bucket_start < ?
	`, int64(fiveMinutes/time.Second), currentHour)
	if err != nil {
		return fmt.Errorf("query telemetry rollup source: %w", err)
	}

	hourly := make(map[string]Bucket)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return fmt.Errorf("scan telemetry rollup source: %w", err)
		}
		var bucket Bucket
		if err := json.Unmarshal([]byte(payload), &bucket); err != nil {
			rows.Close()
			return fmt.Errorf("decode telemetry rollup source: %w", err)
		}
		hourStart := time.Unix(bucket.BucketStart, 0).Truncate(time.Hour).Unix()
		key := fmt.Sprintf("%s\x00%d", bucket.Namespace, hourStart)
		aggregate, ok := hourly[key]
		if !ok {
			aggregate = newBucket(bucket.Namespace, time.Unix(hourStart, 0), oneHour)
		}
		aggregate.Merge(bucket)
		hourly[key] = aggregate
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate telemetry rollup source: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close telemetry rollup source: %w", err)
	}

	rollups := make([]Bucket, 0, len(hourly))
	for _, bucket := range hourly {
		rollups = append(rollups, bucket)
	}
	if err := s.upsert(rollups); err != nil {
		return err
	}
	if _, err := s.db.Exec(`
		DELETE FROM telemetry_buckets
		WHERE (resolution_seconds = ? AND bucket_start < ?)
		   OR (resolution_seconds = ? AND bucket_start < ?)
	`,
		int64(fiveMinutes/time.Second), now.Add(-48*time.Hour).Unix(),
		int64(oneHour/time.Second), now.Add(-31*24*time.Hour).Unix(),
	); err != nil {
		return fmt.Errorf("prune telemetry buckets: %w", err)
	}
	return nil
}
