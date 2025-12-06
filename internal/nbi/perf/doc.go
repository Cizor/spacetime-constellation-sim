// Package perf hosts opt-in performance benchmarks for NBI bulk operations.
//
// The actual benchmarks are behind build tags (`perf`, `perf_large`) so they
// stay out of default test runs, but having this file without tags keeps the
// package discoverable by editors and `go list`.
package perf
