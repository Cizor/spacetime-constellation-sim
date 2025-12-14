package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/model"
)

type samplingClone struct {
	phys         *kb.KnowledgeBase
	net          *core.KnowledgeBase
	motion       *core.MotionModel
	connectivity *core.ConnectivityService
}

func (s *Scheduler) sampleLinkWindows(ctx context.Context, now, horizon time.Time) (map[string][]ContactWindow, error) {
	if horizon.Before(now) {
		return nil, fmt.Errorf("horizon must be >= now")
	}

	snapshot := s.State.Snapshot()
	clone, err := buildSamplingClone(s.State, snapshot)
	if err != nil {
		return nil, err
	}

	if clone == nil {
		return nil, fmt.Errorf("failed to build sampling clone")
	}

	sampleInterval := 30 * time.Second

	windows := make(map[string][]ContactWindow)
	openWindows := make(map[string]time.Time)

	for t := now; !t.After(horizon); t = t.Add(sampleInterval) {
		clone.ensureLinksActive()

		if err := clone.motion.UpdatePositions(t); err != nil {
			return nil, fmt.Errorf("motion update failed: %w", err)
		}
		clone.connectivity.UpdateConnectivity()

		for _, link := range clone.net.GetAllNetworkLinks() {
			visible := link.IsUp
			if visible {
				if _, open := openWindows[link.ID]; !open {
					openWindows[link.ID] = t
				}
				continue
			}
			if start, open := openWindows[link.ID]; open {
				windows[link.ID] = append(windows[link.ID], ContactWindow{
					StartTime: start,
					EndTime:   t,
					Quality:   s.linkSNR(link.ID),
				})
				delete(openWindows, link.ID)
			}
		}
	}

	for linkID, start := range openWindows {
		if !start.IsZero() && start.Before(horizon) {
			windows[linkID] = append(windows[linkID], ContactWindow{
				StartTime: start,
				EndTime:   horizon,
				Quality:   s.linkSNR(linkID),
			})
		}
	}

	return windows, nil
}

func buildSamplingClone(state *state.ScenarioState, snapshot *state.ScenarioSnapshot) (*samplingClone, error) {
	if state == nil || snapshot == nil {
		return nil, fmt.Errorf("state or snapshot is nil")
	}

	phys := kb.NewKnowledgeBase()
	for _, platform := range snapshot.Platforms {
		if platform == nil {
			continue
		}
		if err := phys.AddPlatform(copyPlatform(platform)); err != nil {
			return nil, fmt.Errorf("copy platform %s: %w", platform.ID, err)
		}
	}
	for _, node := range snapshot.Nodes {
		if node == nil {
			continue
		}
		if err := phys.AddNetworkNode(copyNetworkNode(node)); err != nil {
			return nil, fmt.Errorf("copy node %s: %w", node.ID, err)
		}
	}

	net := core.NewKnowledgeBase()
	for _, trx := range state.NetworkKB().ListTransceiverModels() {
		if trx == nil {
			continue
		}
		if err := net.AddTransceiverModel(copyTransceiver(trx)); err != nil {
			return nil, fmt.Errorf("copy transceiver %s: %w", trx.ID, err)
		}
	}
	for _, iface := range snapshot.Interfaces {
		if iface == nil {
			continue
		}
		if err := net.AddInterface(copyInterface(iface)); err != nil {
			return nil, fmt.Errorf("copy interface %s: %w", iface.ID, err)
		}
	}
	for _, link := range snapshot.Links {
		if link == nil {
			continue
		}
		if err := net.AddNetworkLink(copyLink(link)); err != nil {
			return nil, fmt.Errorf("copy link %s: %w", link.ID, err)
		}
	}
	for _, node := range snapshot.Nodes {
		if node == nil {
			continue
		}
		if pos, ok := state.NetworkKB().GetNodeECEFPosition(node.ID); ok {
			net.SetNodeECEFPosition(node.ID, pos)
		}
	}

	motion := core.NewMotionModel(core.WithPositionUpdater(phys))
	for _, platform := range phys.ListPlatforms() {
		if platform == nil {
			continue
		}
		if err := motion.AddPlatform(platform); err != nil {
			return nil, fmt.Errorf("motion add platform %s: %w", platform.ID, err)
		}
	}
	connectivity := core.NewConnectivityService(net)

	return &samplingClone{
		phys:         phys,
		net:          net,
		motion:       motion,
		connectivity: connectivity,
	}, nil
}

func (c *samplingClone) ensureLinksActive() {
	for _, link := range c.net.GetAllNetworkLinks() {
		link.Status = core.LinkStatusActive
	}
}

func copyPlatform(p *model.PlatformDefinition) *model.PlatformDefinition {
	cp := *p
	return &cp
}

func copyNetworkNode(n *model.NetworkNode) *model.NetworkNode {
	cp := *n
	if n.Routes != nil {
		cp.Routes = append([]model.RouteEntry(nil), n.Routes...)
	}
	return &cp
}

func copyInterface(iface *core.NetworkInterface) *core.NetworkInterface {
	cp := *iface
	cp.LinkIDs = append([]string(nil), iface.LinkIDs...)
	return &cp
}

func copyLink(link *core.NetworkLink) *core.NetworkLink {
	cp := *link
	return &cp
}

func copyTransceiver(trx *core.TransceiverModel) *core.TransceiverModel {
	cp := *trx
	return &cp
}
