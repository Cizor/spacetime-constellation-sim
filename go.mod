module github.com/signalsfoundry/constellation-simulator

go 1.24.0

toolchain go1.24.10

require (
	aalyria.com/spacetime v0.0.0
	github.com/joshuaferrara/go-satellite v0.0.0-20220611180459-512638c64e5b
)

require (
	github.com/pkg/errors v0.9.1 // indirect
	google.golang.org/genproto v0.0.0-20251124214823-79d6a2a48846 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251124214823-79d6a2a48846 // indirect
	google.golang.org/grpc v1.77.0 // indirect
	golang.org/x/net v0.46.1-0.20251013234738-63d1a5100f82 // indirect
	golang.org/x/sys v0.37.0 // indirect
	golang.org/x/text v0.30.0 // indirect
	google.golang.org/protobuf v1.36.10
)

replace aalyria.com/spacetime v0.0.0 => ./internal/genproto/aalyria
