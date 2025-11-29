package model

// NetworkNode represents a logical network endpoint.
// In Scope 1 we only care about its link to a PlatformDefinition.
type NetworkNode struct {
    ID   string
    Name string
    Type string // free-form category, e.g. "ROUTER", "UT", etc.

    // PlatformID links this node to a PlatformDefinition.
    // Consumers can obtain the node's position by looking up the platform.
    PlatformID string
}
