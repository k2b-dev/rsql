// Package rsql provides a typed Go client for the rsql HTTP API.
//
// The package separates administrative namespace lifecycle operations from
// namespace-bound database operations. Client values are safe for concurrent
// use and all network operations are controlled by context.Context.
package rsql
