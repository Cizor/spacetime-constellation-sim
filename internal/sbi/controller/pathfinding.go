package controller

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// TimeExpandedNode represents a node at a specific point in time.
type TimeExpandedNode struct {
	NodeID string
	Time   time.Time
}

// TimeExpandedEdge represents a transition between time-expanded nodes.
type TimeExpandedEdge struct {
	From   TimeExpandedNode
	To     TimeExpandedNode
	LinkID string // empty for wait edges
	Cost   int
}

// TimeExpandedGraph contains the nodes and edges that describe time-aware connectivity.
type TimeExpandedGraph struct {
	Nodes []TimeExpandedNode
	Edges []TimeExpandedEdge
}

// BuildTimeExpandedGraph builds a time-expanded graph between srcNodeID and dstNodeID.
// It uses the scheduler's cached contact windows to create link and wait edges.
func (s *Scheduler) BuildTimeExpandedGraph(ctx context.Context, srcNodeID, dstNodeID string, startTime, endTime time.Time) (*TimeExpandedGraph, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if srcNodeID == "" || dstNodeID == "" {
		return nil, fmt.Errorf("source and destination node IDs must be provided")
	}
	if startTime.After(endTime) {
		return nil, fmt.Errorf("startTime must be before endTime")
	}

	s.ensureContactWindows(ctx)

	interfacesByNode := s.State.InterfacesByNode()
	linkEndpoints := map[string][2]string{}
	for _, link := range s.State.ListLinks() {
		if link == nil {
			continue
		}
		ifaceA := findInterfaceByRef(interfacesByNode, link.InterfaceA)
		ifaceB := findInterfaceByRef(interfacesByNode, link.InterfaceB)
		if ifaceA == nil || ifaceB == nil {
			continue
		}
		if ifaceA.ParentNodeID == "" || ifaceB.ParentNodeID == "" {
			continue
		}
		linkEndpoints[link.ID] = [2]string{ifaceA.ParentNodeID, ifaceB.ParentNodeID}
	}

	timePoints := make(map[string]map[time.Time]struct{})
	addTimePoint := func(nodeID string, t time.Time) {
		if nodeID == "" {
			return
		}
		if t.Before(startTime) || t.After(endTime) {
			return
		}
		if _, ok := timePoints[nodeID]; !ok {
			timePoints[nodeID] = make(map[time.Time]struct{})
		}
		timePoints[nodeID][t] = struct{}{}
	}

	addTimePoint(srcNodeID, startTime)
	addTimePoint(dstNodeID, startTime)
	addTimePoint(dstNodeID, endTime)

	type linkWindow struct {
		linkID string
		a      string
		b      string
		start  time.Time
		end    time.Time
	}
	linkWindows := []linkWindow{}

	for linkID, windows := range s.contactWindows {
		endpoints, ok := linkEndpoints[linkID]
		if !ok {
			continue
		}
		for _, window := range windows {
			if window.EndTime.Before(startTime) {
				continue
			}
			if window.StartTime.After(endTime) {
				continue
			}
			windowStart := window.StartTime
			if windowStart.Before(startTime) {
				windowStart = startTime
			}
			windowEnd := window.EndTime
			if windowEnd.After(endTime) {
				windowEnd = endTime
			}
			if !windowEnd.After(windowStart) {
				continue
			}
			addTimePoint(endpoints[0], windowStart)
			addTimePoint(endpoints[1], windowStart)
			addTimePoint(endpoints[0], windowEnd)
			addTimePoint(endpoints[1], windowEnd)
			linkWindows = append(linkWindows, linkWindow{
				linkID: linkID,
				a:      endpoints[0],
				b:      endpoints[1],
				start:  windowStart,
				end:    windowEnd,
			})
		}
	}

	nodeSnapshot := make(map[string]map[time.Time]TimeExpandedNode)
	graph := &TimeExpandedGraph{}
	for nodeID, points := range timePoints {
		times := make([]time.Time, 0, len(points))
		for t := range points {
			times = append(times, t)
		}
		sort.Slice(times, func(i, j int) bool {
			return times[i].Before(times[j])
		})
		nodeSnapshot[nodeID] = make(map[time.Time]TimeExpandedNode)
		for _, t := range times {
			node := TimeExpandedNode{NodeID: nodeID, Time: t}
			graph.Nodes = append(graph.Nodes, node)
			nodeSnapshot[nodeID][t] = node
		}
	}

	for nodeID, pts := range nodeSnapshot {
		times := make([]time.Time, 0, len(pts))
		for t := range pts {
			times = append(times, t)
		}
		sort.Slice(times, func(i, j int) bool {
			return times[i].Before(times[j])
		})
		for i := 0; i < len(times)-1; i++ {
			from := nodeSnapshot[nodeID][times[i]]
			to := nodeSnapshot[nodeID][times[i+1]]
			duration := to.Time.Sub(from.Time)
			cost := int(math.Max(1, math.Ceil(duration.Seconds())))
			graph.Edges = append(graph.Edges, TimeExpandedEdge{
				From:   from,
				To:     to,
				LinkID: "",
				Cost:   cost,
			})
		}
	}

	for _, lw := range linkWindows {
		aNode, aOK := nodeSnapshot[lw.a][lw.start]
		bNode, bOK := nodeSnapshot[lw.b][lw.end]
		if aOK && bOK {
			graph.Edges = append(graph.Edges, TimeExpandedEdge{
				From:   aNode,
				To:     bNode,
				LinkID: lw.linkID,
				Cost:   timeCost(lw.end.Sub(lw.start)),
			})
		}
		bNode, bOK = nodeSnapshot[lw.b][lw.start]
		aNode, aOK = nodeSnapshot[lw.a][lw.end]
		if aOK && bOK {
			graph.Edges = append(graph.Edges, TimeExpandedEdge{
				From:   bNode,
				To:     aNode,
				LinkID: lw.linkID,
				Cost:   timeCost(lw.end.Sub(lw.start)),
			})
		}
	}

	return graph, nil
}

func timeCost(d time.Duration) int {
	seconds := d.Seconds()
	if seconds <= 0 {
		return 1
	}
	return int(math.Max(1, math.Ceil(seconds)))
}
