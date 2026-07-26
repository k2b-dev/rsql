package service

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/ValentinKolb/rsql/internal/domain"
	"github.com/ValentinKolb/rsql/internal/store/sqlite"
	"github.com/ValentinKolb/rsql/internal/telemetry"
)

var overviewWindows = map[string]time.Duration{
	"1h":  time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// RecordRequest records one completed namespace API request in memory.
func (s *Service) RecordRequest(event telemetry.Event) {
	s.telemetry.Record(event)
}

// Overview returns a live and historical namespace operational summary.
func (s *Service) Overview(namespace, windowName string) (domain.NamespaceOverview, error) {
	duration, ok := overviewWindows[windowName]
	if !ok {
		return domain.NamespaceOverview{}, domain.NewError(
			domain.ErrInvalidRequest,
			400,
			"window must be one of 1h, 24h, 7d, or 30d",
		)
	}
	record, err := s.registry.Get(namespace)
	if err != nil {
		if err == sql.ErrNoRows {
			return domain.NamespaceOverview{}, domain.NewError(
				domain.ErrNamespaceNotFound,
				404,
				fmt.Sprintf("Namespace '%s' does not exist", namespace),
			)
		}
		return domain.NamespaceOverview{}, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace", err)
	}
	config, err := s.GetNamespaceConfig(namespace)
	if err != nil {
		return domain.NamespaceOverview{}, err
	}

	var storage sqlite.StorageInfo
	var schema domain.OverviewSchema
	err = s.ns.WithRead(namespace, record.DBPath, func(db *sql.DB) error {
		var readErr error
		storage, readErr = sqlite.Storage(db, record.DBPath, config.MaxDBSize)
		if readErr != nil {
			return readErr
		}
		schema, readErr = sqlite.SchemaSummary(db)
		return readErr
	})
	if err != nil {
		return domain.NamespaceOverview{}, mapErr(err)
	}

	s.telemetry.RecordStorage(namespace, telemetry.StorageSnapshot{
		UsedBytes:      storage.UsedBytes,
		AllocatedBytes: storage.AllocatedBytes,
		WALBytes:       storage.WALBytes,
		DiskBytes:      storage.DiskBytes,
	})

	now := time.Now().UTC()
	from := now.Add(-duration)
	buckets, err := s.telemetry.Buckets(namespace, from, now.Add(time.Second))
	if err != nil {
		return domain.NamespaceOverview{}, domain.WrapError(domain.ErrInternal, 500, "failed to read namespace telemetry", err)
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].BucketStart < buckets[j].BucketStart
	})

	aggregate := telemetry.Bucket{}
	series := make([]domain.OverviewActivityPoint, 0, len(buckets))
	for _, bucket := range buckets {
		aggregate.Merge(bucket)
		series = append(series, domain.OverviewActivityPoint{
			Timestamp: time.Unix(bucket.BucketStart, 0).UTC().Format(time.RFC3339),
			Reads:     bucket.Reads,
			Writes:    bucket.Writes,
			Errors:    bucket.Errors,
		})
	}
	requests := aggregate.Reads + aggregate.Writes
	errorRate := ratio(aggregate.Errors, requests)
	var lastActivity *string
	if aggregate.LastActivityAt > 0 {
		value := time.Unix(aggregate.LastActivityAt, 0).UTC().Format(time.RFC3339)
		lastActivity = &value
	}

	mode := "read_write"
	if config.ReadOnly {
		mode = "read_only"
	}
	issues := overviewIssues(storage, requests, errorRate, aggregate)
	status := "healthy"
	if len(issues) > 0 {
		status = "degraded"
	}

	resolution := "5m"
	if duration > 48*time.Hour {
		resolution = "1h"
	}
	return domain.NamespaceOverview{
		GeneratedAt: now.Format(time.RFC3339),
		Window: domain.OverviewWindow{
			Name:       windowName,
			From:       from.Format(time.RFC3339),
			To:         now.Format(time.RFC3339),
			Resolution: resolution,
		},
		Health: domain.OverviewHealth{Status: status, Mode: mode, Issues: issues},
		Storage: domain.OverviewStorage{
			UsedBytes:         storage.UsedBytes,
			AllocatedBytes:    storage.AllocatedBytes,
			ReclaimableBytes:  storage.ReclaimableBytes,
			QuotaBytes:        storage.QuotaBytes,
			RemainingBytes:    storage.RemainingBytes,
			UsageRatio:        storage.UsageRatio,
			DatabaseFileBytes: storage.DatabaseFileBytes,
			WALBytes:          storage.WALBytes,
			SHMBytes:          storage.SHMBytes,
			DiskBytes:         storage.DiskBytes,
		},
		Activity: domain.OverviewActivity{
			Requests:       requests,
			Reads:          aggregate.Reads,
			Writes:         aggregate.Writes,
			Errors:         aggregate.Errors,
			ErrorRate:      errorRate,
			ResponseBytes:  aggregate.ResponseBytes,
			LastActivityAt: lastActivity,
			Series:         series,
		},
		ReadLatency:  overviewLatency(aggregate.ReadDuration),
		WriteLatency: overviewLatency(aggregate.WriteDuration),
		WriteContention: domain.OverviewWriteContention{
			Wait:         overviewLatency(aggregate.WriteWait),
			BusyErrors:   aggregate.BusyErrors,
			Timeouts:     aggregate.Timeouts,
			QuotaRejects: aggregate.QuotaRejected,
		},
		Schema:        schema,
		Realtime:      domain.OverviewRealtime{Subscribers: s.broker.Count(namespace)},
		TopOperations: topOperations(aggregate.Operations),
		Growth:        overviewGrowth(buckets, storage),
	}, nil
}

func overviewLatency(histogram telemetry.Histogram) domain.OverviewLatency {
	return domain.OverviewLatency{
		Count:     histogram.Total,
		AverageMS: histogram.AverageMS(),
		P50MS:     histogram.PercentileMS(0.50),
		P95MS:     histogram.PercentileMS(0.95),
		P99MS:     histogram.PercentileMS(0.99),
	}
}

func topOperations(operations map[string]telemetry.OperationAggregate) []domain.OverviewOperation {
	out := make([]domain.OverviewOperation, 0, len(operations))
	for name, operation := range operations {
		out = append(out, domain.OverviewOperation{
			Name:      name,
			Count:     operation.Count,
			Errors:    operation.Errors,
			ErrorRate: ratio(operation.Errors, operation.Count),
			Latency:   overviewLatency(operation.Duration),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Name < out[j].Name
		}
		return out[i].Count > out[j].Count
	})
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}

func overviewIssues(storage sqlite.StorageInfo, requests uint64, errorRate float64, aggregate telemetry.Bucket) []string {
	issues := make([]string, 0, 3)
	if storage.UsageRatio != nil && *storage.UsageRatio >= 0.90 {
		issues = append(issues, "storage_near_quota")
	}
	if requests >= 20 && errorRate >= 0.05 {
		issues = append(issues, "elevated_error_rate")
	}
	if aggregate.BusyErrors > 0 || aggregate.WriteWait.PercentileMS(0.95) >= 100 {
		issues = append(issues, "write_contention")
	}
	return issues
}

func overviewGrowth(buckets []telemetry.Bucket, storage sqlite.StorageInfo) *domain.OverviewGrowth {
	type sample struct {
		at    int64
		bytes int64
	}
	samples := make([]sample, 0, len(buckets))
	for _, bucket := range buckets {
		if bucket.Storage != nil {
			samples = append(samples, sample{at: bucket.BucketStart, bytes: bucket.Storage.AllocatedBytes})
		}
	}
	if len(samples) < 2 {
		return nil
	}
	first, last := samples[0], samples[len(samples)-1]
	elapsedDays := time.Unix(last.at, 0).Sub(time.Unix(first.at, 0)).Hours() / 24
	if elapsedDays <= 0 {
		return nil
	}
	bytesPerDay := float64(last.bytes-first.bytes) / elapsedDays
	var projected *string
	if bytesPerDay > 0 && storage.QuotaBytes > storage.AllocatedBytes {
		days := float64(storage.QuotaBytes-storage.AllocatedBytes) / bytesPerDay
		value := time.Now().UTC().Add(time.Duration(days * float64(24*time.Hour))).Format(time.RFC3339)
		projected = &value
	}
	return &domain.OverviewGrowth{
		BytesPerDay:     bytesPerDay,
		SampleCount:     len(samples),
		ProjectedFullAt: projected,
	}
}

func ratio(numerator, denominator uint64) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
