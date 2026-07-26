// Package telemetry records bounded, namespace-scoped usage aggregates.
//
// Request handlers only update in-memory buckets. A background flush persists
// dirty buckets to telemetry.db and rolls completed buckets up for retention.
package telemetry
