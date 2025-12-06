//go:build perf

package perf

import "testing"

var smallConfig = perfConfig{
	Platforms:         1000,
	Nodes:             1000,
	InterfacesPerNode: 2,
	Links:             1000,
	ServiceRequests:   1000,
}

func BenchmarkPlatformCreateSmall(b *testing.B) {
	benchmarkPlatforms(b, smallConfig)
}

func BenchmarkNodeCreateSmall(b *testing.B) {
	benchmarkNodes(b, smallConfig)
}

func BenchmarkLinkCreateSmall(b *testing.B) {
	benchmarkLinks(b, smallConfig)
}

func BenchmarkServiceRequestCreateSmall(b *testing.B) {
	benchmarkServiceRequests(b, smallConfig)
}
