// Package store defines the persistence contract for metric data points.
package store

import "time"

// Point is a single collected metric data point.
type Point struct {
	Name      string    // metric name
	Type      string    // metric type: counter or gauge
	Value     float64   // collected value
	Timestamp time.Time // collection time in UTC
}

// Store persists and retrieves metric data points.
type Store interface {
	// Write durably persists a data point.
	Write(p Point) error
	// Query returns points for the metric within [from, to] inclusive,
	// ordered by timestamp ascending. Unknown metrics return an empty slice.
	Query(name string, from, to time.Time) ([]Point, error)
	// Close flushes pending writes and releases resources.
	Close() error
}
