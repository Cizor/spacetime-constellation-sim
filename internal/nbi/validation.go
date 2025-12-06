package nbi

import (
	"errors"
	"fmt"
	"strings"
	"time"

	common "aalyria.com/spacetime/api/common"
	resources "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

var (
	ErrInvalidPlatform       = errors.New("invalid platform")
	ErrInvalidNode           = errors.New("invalid node")
	ErrInvalidInterface      = errors.New("invalid interface")
	ErrInvalidLink           = errors.New("invalid link")
	ErrInvalidServiceRequest = errors.New("invalid service request")
)

// ValidatePlatformProto performs basic structural validation for a platform.
func ValidatePlatformProto(pd *common.PlatformDefinition) error {
	if pd == nil {
		return fmt.Errorf("%w: platform definition is required", ErrInvalidPlatform)
	}
	if strings.TrimSpace(pd.GetName()) == "" {
		return fmt.Errorf("%w: name (used as ID) is required", ErrInvalidPlatform)
	}
	if strings.TrimSpace(pd.GetType()) == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidPlatform)
	}

	// Orbital platforms must specify a motion source (e.g. TLE-backed propagation).
	if strings.EqualFold(pd.GetType(), "SATELLITE") && pd.GetMotionSource() == common.PlatformDefinition_UNKNOWN_SOURCE {
		return fmt.Errorf("%w: motion source is required for orbital platforms", ErrInvalidPlatform)
	}

	return nil
}

// ValidateInterfaceProto checks a single interface for required fields.
func ValidateInterfaceProto(iface *resources.NetworkInterface) error {
	if iface == nil {
		return fmt.Errorf("%w: interface is required", ErrInvalidInterface)
	}
	if strings.TrimSpace(iface.GetInterfaceId()) == "" {
		return fmt.Errorf("%w: interface_id is required", ErrInvalidInterface)
	}

	switch medium := iface.GetInterfaceMedium().(type) {
	case *resources.NetworkInterface_Wired:
		if medium.Wired == nil {
			return fmt.Errorf("%w: wired interface details are required", ErrInvalidInterface)
		}
	case *resources.NetworkInterface_Wireless:
		if medium.Wireless == nil {
			return fmt.Errorf("%w: wireless interface details are required", ErrInvalidInterface)
		}
		trxID := ""
		if medium.Wireless.GetTransceiverModelId() != nil {
			trxID = medium.Wireless.GetTransceiverModelId().GetTransceiverModelId()
		}
		if strings.TrimSpace(trxID) == "" {
			return fmt.Errorf("%w: wireless interface %q missing transceiver_model_id", ErrInvalidInterface, iface.GetInterfaceId())
		}
	default:
		return fmt.Errorf("%w: interface_medium is required", ErrInvalidInterface)
	}

	return nil
}

// ValidateNodeProto checks a NetworkNode and its embedded interfaces.
func ValidateNodeProto(node *resources.NetworkNode) error {
	if node == nil {
		return fmt.Errorf("%w: node is required", ErrInvalidNode)
	}
	if strings.TrimSpace(node.GetNodeId()) == "" {
		return fmt.Errorf("%w: node_id is required", ErrInvalidNode)
	}

	if len(node.GetNodeInterface()) == 0 {
		return fmt.Errorf("%w: at least one interface is required", ErrInvalidNode)
	}

	if _, err := platformIDFromInterfaces(node); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidNode, err)
	}

	seen := make(map[string]struct{}, len(node.GetNodeInterface()))
	for i, iface := range node.GetNodeInterface() {
		if iface == nil {
			return fmt.Errorf("%w: interface[%d] is nil", ErrInvalidNode, i)
		}
		if err := ValidateInterfaceProto(iface); err != nil {
			return fmt.Errorf("%w: interface[%d]: %v", ErrInvalidNode, i, err)
		}

		parent, local := splitInterfaceRef(iface.GetInterfaceId())
		if parent != "" && parent != node.GetNodeId() {
			return fmt.Errorf("%w: interface %q belongs to different node %q", ErrInvalidNode, iface.GetInterfaceId(), parent)
		}
		if local == "" {
			return fmt.Errorf("%w: interface[%d] id is empty", ErrInvalidNode, i)
		}
		if _, ok := seen[local]; ok {
			return fmt.Errorf("%w: duplicate interface_id %q for node %q", ErrInvalidNode, local, node.GetNodeId())
		}
		seen[local] = struct{}{}
	}

	return nil
}

// ValidateLinkProto enforces structural sanity for a bidirectional link.
func ValidateLinkProto(link *resources.BidirectionalLink) error {
	if link == nil {
		return fmt.Errorf("%w: link is required", ErrInvalidLink)
	}

	endA := extractBidirectionalEndpoint(
		link.GetANetworkNodeId(),
		link.GetATxInterfaceId(),
		link.GetARxInterfaceId(),
		link.GetA(),
	)
	endB := extractBidirectionalEndpoint(
		link.GetBNetworkNodeId(),
		link.GetBTxInterfaceId(),
		link.GetBRxInterfaceId(),
		link.GetB(),
	)

	aIface := normalizeInterfaceRef(endA.nodeID, endA.txInterface())
	if aIface == "" {
		aIface = normalizeInterfaceRef(endA.nodeID, endA.rxInterface())
	}
	bIface := normalizeInterfaceRef(endB.nodeID, endB.txInterface())
	if bIface == "" {
		bIface = normalizeInterfaceRef(endB.nodeID, endB.rxInterface())
	}

	if aIface == "" || bIface == "" {
		return fmt.Errorf("%w: both link endpoints must specify node and interface IDs", ErrInvalidLink)
	}
	if aIface == bIface {
		return fmt.Errorf("%w: link endpoints must be distinct", ErrInvalidLink)
	}

	return nil
}

// ValidateServiceRequestProto checks structural validity for a ServiceRequest.
func ValidateServiceRequestProto(sr *resources.ServiceRequest) error {
	if sr == nil {
		return fmt.Errorf("%w: service request is required", ErrInvalidServiceRequest)
	}
	if strings.TrimSpace(sr.GetSrcNodeId()) == "" || strings.TrimSpace(sr.GetDstNodeId()) == "" {
		return fmt.Errorf("%w: src_node_id and dst_node_id are required", ErrInvalidServiceRequest)
	}

	if len(sr.GetRequirements()) == 0 {
		return fmt.Errorf("%w: at least one flow requirement is required", ErrInvalidServiceRequest)
	}

	for i, req := range sr.GetRequirements() {
		if req == nil {
			return fmt.Errorf("%w: flow requirement %d is nil", ErrInvalidServiceRequest, i)
		}
		if req.GetBandwidthBpsRequested() < 0 {
			return fmt.Errorf("%w: flow requirement %d requested bandwidth cannot be negative", ErrInvalidServiceRequest, i)
		}
		if req.GetBandwidthBpsMinimum() < 0 {
			return fmt.Errorf("%w: flow requirement %d minimum bandwidth cannot be negative", ErrInvalidServiceRequest, i)
		}
		if req.GetLatencyMaximum() != nil && req.GetLatencyMaximum().AsDuration() < 0 {
			return fmt.Errorf("%w: flow requirement %d latency cannot be negative", ErrInvalidServiceRequest, i)
		}
		if ti := req.GetTimeInterval(); ti != nil {
			start := protoTimeToTime(ti.GetStartTime())
			end := protoTimeToTime(ti.GetEndTime())
			if !start.IsZero() && !end.IsZero() && end.Before(start) {
				return fmt.Errorf("%w: flow requirement %d has invalid time interval (end before start)", ErrInvalidServiceRequest, i)
			}
		}
	}

	return nil
}

// platformIDFromInterfaces extracts a single platform_id from the node's
// interfaces. If multiple non-empty platform_ids are present and disagree, an
// error is returned.
func platformIDFromInterfaces(in *resources.NetworkNode) (string, error) {
	var platformID string

	setOrVerify := func(candidate string) error {
		if candidate == "" {
			return nil
		}
		if platformID == "" {
			platformID = candidate
			return nil
		}
		if platformID != candidate {
			return fmt.Errorf("conflicting platform_id values: %q vs %q", platformID, candidate)
		}
		return nil
	}

	for _, iface := range in.GetNodeInterface() {
		if iface == nil {
			continue
		}

		switch medium := iface.GetInterfaceMedium().(type) {
		case *resources.NetworkInterface_Wired:
			if medium.Wired != nil {
				if err := setOrVerify(medium.Wired.GetPlatformId()); err != nil {
					return "", err
				}
			}
		case *resources.NetworkInterface_Wireless:
			if medium.Wireless != nil {
				if err := setOrVerify(medium.Wireless.GetPlatform()); err != nil {
					return "", err
				}
			}
		}
	}

	return platformID, nil
}

func splitInterfaceRef(ref string) (string, string) {
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", ref
}

// normalizeInterfaceRef ensures we have a "node/iface" form.
// If nodeID is empty, it will be inferred from ifaceID if possible.
func normalizeInterfaceRef(nodeID, ifaceID string) string {
	if nodeID == "" {
		if n, local := splitInterfaceRef(ifaceID); n != "" {
			nodeID = n
			ifaceID = local
		}
	}
	switch {
	case nodeID == "" && ifaceID == "":
		return ""
	case nodeID == "":
		return ifaceID
	case ifaceID == "":
		return nodeID
	default:
		return nodeID + "/" + ifaceID
	}
}

type bidirectionalEndpoint struct {
	nodeID string
	txID   string
	rxID   string
}

func extractBidirectionalEndpoint(nodeID, txID, rxID string, end *resources.LinkEnd) bidirectionalEndpoint {
	if end != nil && end.Id != nil {
		if nodeID == "" {
			nodeID = end.Id.GetNodeId()
		}
		if txID == "" {
			txID = end.Id.GetInterfaceId()
		}
		if rxID == "" {
			rxID = end.Id.GetInterfaceId()
		}
	}

	return bidirectionalEndpoint{
		nodeID: nodeID,
		txID:   txID,
		rxID:   rxID,
	}
}

func (e bidirectionalEndpoint) txInterface() string {
	if e.txID != "" {
		return e.txID
	}
	return e.rxID
}

func (e bidirectionalEndpoint) rxInterface() string {
	if e.rxID != "" {
		return e.rxID
	}
	return e.txID
}

func protoTimeToTime(dt *common.DateTime) time.Time {
	if dt == nil {
		return time.Time{}
	}
	return time.UnixMicro(dt.GetUnixTimeUsec())
}
