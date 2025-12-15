package model

// SchedulingDomain represents an administrative/federated domain.
type SchedulingDomain struct {
	DomainID           string
	Name               string
	Nodes              []string
	Capabilities       map[string]interface{}
	FederationEndpoint string
}
