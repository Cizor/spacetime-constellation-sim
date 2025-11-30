# Aalyria Spacetime API protos

This directory vendors a subset of Aalyria's Spacetime API proto files, taken
from the `api-main` repository (Northbound Interface + common types) for use
by the spacetime-constellation-sim project.

Layout mirrors the upstream imports, e.g.:

- `api/common/platform.proto`
- `api/nbi/v1alpha/resources/network_element.proto`
- `api/nbi/v1alpha/nbi.proto`

These are used for:
- PlatformDefinition
- NetworkNode / NetworkInterface
- NetworkLink / BidirectionalLink
- ServiceRequest
- NBI service definitions
