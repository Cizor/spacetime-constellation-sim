//go:build perf_large

package perf

import "testing"

var largeConfig = perfConfig{
	Platforms:         3000,
	Nodes:             3000,
	InterfacesPerNode: 2,
	Links:             3000,
	ServiceRequests:   3000,
}

func BenchmarkPlatformCreateLarge(b *testing.B) {
	benchmarkPlatforms(b, largeConfig)
}

func BenchmarkNodeCreateLarge(b *testing.B) {
	benchmarkNodes(b, largeConfig)
}

func BenchmarkLinkCreateLarge(b *testing.B) {
	benchmarkLinks(b, largeConfig)
}

func BenchmarkServiceRequestCreateLarge(b *testing.B) {
	benchmarkServiceRequests(b, largeConfig)
}
