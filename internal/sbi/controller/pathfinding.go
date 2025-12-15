package controller

import (
	"container/heap"
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

// DTNHop describes a link traversal within a store-and-forward path.
type DTNHop struct {
	FromNodeID      string
	ToNodeID        string
	LinkID          string
	StartTime       time.Time
	EndTime         time.Time
	StorageNodeID   string
	StorageAt       string
	StorageStart    time.Time
	StorageDuration time.Duration
	StorageEnd      time.Time
}

// DTNPath captures the store-and-forward path taken by a message.
type DTNPath struct {
	Hops         []DTNHop
	StorageNodes []string
	TotalDelay   time.Duration
}

// Path represents a sequence of hops over time-aware links.
type Path struct {
	Hops         []PathHop
	TotalLatency time.Duration
	ValidFrom    time.Time
	ValidUntil   time.Time
}

// PathDiff describes shared, added, and removed hops when comparing paths.
type PathDiff struct {
	SharedHops  []PathHop
	RemovedHops []PathHop
	AddedHops   []PathHop
}

// PathHop describes one link traversal.
type PathHop struct {
	FromNodeID string
	ToNodeID   string
	LinkID     string
	StartTime  time.Time
	EndTime    time.Time
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

var (
	// ErrPathNotFound indicates there is no path within the given horizon.
	ErrPathNotFound = fmt.Errorf("no multi-hop path found")
)

// FindMultiHopPath finds a multi-hop route between the given nodes within the specified horizon.
func (s *Scheduler) FindMultiHopPath(ctx context.Context, srcNodeID, dstNodeID string, startTime time.Time, horizon time.Duration) (*Path, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if srcNodeID == "" || dstNodeID == "" {
		return nil, fmt.Errorf("source and destination nodes must be provided")
	}
	if horizon <= 0 {
		return nil, fmt.Errorf("horizon must be positive")
	}

	endTime := startTime.Add(horizon)
	graph, err := s.BuildTimeExpandedGraph(ctx, srcNodeID, dstNodeID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	if len(graph.Nodes) == 0 {
		return nil, ErrPathNotFound
	}

	nodeIdx := make(map[TimeExpandedNode]int)
	for i, node := range graph.Nodes {
		nodeIdx[node] = i
	}

	adj := make([][]int, len(graph.Nodes))
	for ei, edge := range graph.Edges {
		fromIdx, ok := nodeIdx[edge.From]
		if !ok {
			continue
		}
		adj[fromIdx] = append(adj[fromIdx], ei)
	}

	startNodes := []int{}
	for idx, node := range graph.Nodes {
		if node.NodeID == srcNodeID && !node.Time.Before(startTime) {
			startNodes = append(startNodes, idx)
		}
	}
	if len(startNodes) == 0 {
		return nil, ErrPathNotFound
	}

	destCandidates := make(map[int]struct{})
	for idx, node := range graph.Nodes {
		if node.NodeID == dstNodeID && !node.Time.Before(startTime) && !node.Time.After(endTime) {
			destCandidates[idx] = struct{}{}
		}
	}
	if len(destCandidates) == 0 {
		return nil, ErrPathNotFound
	}

	dist := make([]int, len(graph.Nodes))
	prevEdge := make([]int, len(graph.Nodes))
	for i := range dist {
		dist[i] = math.MaxInt32
		prevEdge[i] = -1
	}

	pq := &dijkstraQueue{}
	heap.Init(pq)
	for _, idx := range startNodes {
		dist[idx] = 0
		heap.Push(pq, dijkstraNode{nodeIdx: idx, cost: 0})
	}

	var destIdx = -1
	for pq.Len() > 0 {
		state := heap.Pop(pq).(dijkstraNode)
		if state.cost > dist[state.nodeIdx] {
			continue
		}
		if _, ok := destCandidates[state.nodeIdx]; ok {
			destIdx = state.nodeIdx
			break
		}
		for _, ei := range adj[state.nodeIdx] {
			edge := graph.Edges[ei]
			toIdx, ok := nodeIdx[edge.To]
			if !ok {
				continue
			}
			newCost := state.cost + edge.Cost
			if newCost < dist[toIdx] {
				dist[toIdx] = newCost
				prevEdge[toIdx] = ei
				heap.Push(pq, dijkstraNode{nodeIdx: toIdx, cost: newCost})
			}
		}
	}

	if destIdx == -1 || dist[destIdx] == math.MaxInt32 {
		return nil, ErrPathNotFound
	}

	var edgePath []TimeExpandedEdge
	current := destIdx
	for prevEdge[current] != -1 {
		edge := graph.Edges[prevEdge[current]]
		edgePath = append(edgePath, edge)
		fromIdx, ok := nodeIdx[edge.From]
		if !ok || fromIdx == current {
			break
		}
		current = fromIdx
	}

	if len(edgePath) == 0 {
		return nil, ErrPathNotFound
	}

	// Reverse edgePath
	for i, j := 0, len(edgePath)-1; i < j; i, j = i+1, j-1 {
		edgePath[i], edgePath[j] = edgePath[j], edgePath[i]
	}

	var path Path
	for _, edge := range edgePath {
		if edge.LinkID == "" {
			continue
		}
		hop := PathHop{
			FromNodeID: edge.From.NodeID,
			ToNodeID:   edge.To.NodeID,
			LinkID:     edge.LinkID,
			StartTime:  edge.From.Time,
			EndTime:    edge.To.Time,
		}
		path.Hops = append(path.Hops, hop)
		path.TotalLatency += hop.EndTime.Sub(hop.StartTime)
		if path.ValidFrom.IsZero() || hop.StartTime.Before(path.ValidFrom) {
			path.ValidFrom = hop.StartTime
		}
		if hop.EndTime.After(path.ValidUntil) {
			path.ValidUntil = hop.EndTime
		}
	}

	if len(path.Hops) == 0 {
		return nil, ErrPathNotFound
	}

	return &path, nil
}

// FindDTNPath finds a store-and-forward route considering DTN storage constraints.
func (s *Scheduler) FindDTNPath(ctx context.Context, srcNodeID, dstNodeID string, msgSize uint64, startTime time.Time) (*DTNPath, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if srcNodeID == "" || dstNodeID == "" {
		return nil, fmt.Errorf("source and destination nodes must be provided")
	}
	if msgSize == 0 {
		// zero-size messages do not consume storage, treat as standard pathfinding.
		msgSize = 0
	}
	if startTime.IsZero() && s.Clock != nil {
		startTime = s.Clock.Now()
	}

	endTime := startTime.Add(ContactHorizon)
	graph, err := s.BuildTimeExpandedGraph(ctx, srcNodeID, dstNodeID, startTime, endTime)
	if err != nil {
		return nil, err
	}

	if len(graph.Nodes) == 0 {
		return nil, ErrPathNotFound
	}

	nodeIdx := make(map[TimeExpandedNode]int)
	for i, node := range graph.Nodes {
		nodeIdx[node] = i
	}

	adj := make([][]int, len(graph.Nodes))
	allowWait := func(nodeID string) bool {
		if msgSize == 0 {
			return true
		}
		return s.canStoreMessage(nodeID, msgSize)
	}
	for ei, edge := range graph.Edges {
		if edge.LinkID == "" && !allowWait(edge.From.NodeID) {
			continue
		}
		fromIdx, ok := nodeIdx[edge.From]
		if !ok {
			continue
		}
		adj[fromIdx] = append(adj[fromIdx], ei)
	}

	startNodes := []int{}
	for idx, node := range graph.Nodes {
		if node.NodeID == srcNodeID && !node.Time.Before(startTime) {
			startNodes = append(startNodes, idx)
		}
	}
	if len(startNodes) == 0 {
		return nil, ErrPathNotFound
	}

	destCandidates := make(map[int]struct{})
	for idx, node := range graph.Nodes {
		if node.NodeID == dstNodeID && !node.Time.Before(startTime) && !node.Time.After(endTime) {
			destCandidates[idx] = struct{}{}
		}
	}
	if len(destCandidates) == 0 {
		return nil, ErrPathNotFound
	}

	dist := make([]int, len(graph.Nodes))
	prevEdge := make([]int, len(graph.Nodes))
	for i := range dist {
		dist[i] = math.MaxInt32
		prevEdge[i] = -1
	}

	pq := &dijkstraQueue{}
	heap.Init(pq)
	for _, idx := range startNodes {
		dist[idx] = 0
		heap.Push(pq, dijkstraNode{nodeIdx: idx, cost: 0})
	}

	var destIdx = -1
	for pq.Len() > 0 {
		state := heap.Pop(pq).(dijkstraNode)
		if state.cost > dist[state.nodeIdx] {
			continue
		}
		if _, ok := destCandidates[state.nodeIdx]; ok {
			destIdx = state.nodeIdx
			break
		}
		for _, ei := range adj[state.nodeIdx] {
			edge := graph.Edges[ei]
			toIdx, ok := nodeIdx[edge.To]
			if !ok {
				continue
			}
			newCost := state.cost + edge.Cost
			if newCost < dist[toIdx] {
				dist[toIdx] = newCost
				prevEdge[toIdx] = ei
				heap.Push(pq, dijkstraNode{nodeIdx: toIdx, cost: newCost})
			}
		}
	}

	if destIdx == -1 || dist[destIdx] == math.MaxInt32 {
		return nil, ErrPathNotFound
	}

	var edgePath []TimeExpandedEdge
	current := destIdx
	for prevEdge[current] != -1 {
		edge := graph.Edges[prevEdge[current]]
		edgePath = append(edgePath, edge)
		fromIdx, ok := nodeIdx[edge.From]
		if !ok || fromIdx == current {
			break
		}
		current = fromIdx
	}

	if len(edgePath) == 0 {
		return nil, ErrPathNotFound
	}

	for i, j := 0, len(edgePath)-1; i < j; i, j = i+1, j-1 {
		edgePath[i], edgePath[j] = edgePath[j], edgePath[i]
	}

	path := &DTNPath{}
	storageSet := make(map[string]struct{})
	var storageNode string
	var storageDuration time.Duration
	var storageStart time.Time
	var storageEnd time.Time
	for _, edge := range edgePath {
		if edge.LinkID == "" {
			if storageNode == "" {
				storageNode = edge.From.NodeID
				storageStart = edge.From.Time
			}
			storageDuration += edge.To.Time.Sub(edge.From.Time)
			storageEnd = edge.To.Time
			storageSet[storageNode] = struct{}{}
			continue
		}
		hop := DTNHop{
			FromNodeID:      edge.From.NodeID,
			ToNodeID:        edge.To.NodeID,
			LinkID:          edge.LinkID,
			StartTime:       edge.From.Time,
			EndTime:         edge.To.Time,
			StorageNodeID:   storageNode,
			StorageAt:       storageNode,
			StorageStart:    storageStart,
			StorageDuration: storageDuration,
			StorageEnd:      storageEnd,
		}
		path.Hops = append(path.Hops, hop)
		storageNode = ""
		storageDuration = 0
		storageStart = time.Time{}
		storageEnd = time.Time{}
	}

	if len(path.Hops) == 0 {
		return nil, ErrPathNotFound
	}

	for node := range storageSet {
		path.StorageNodes = append(path.StorageNodes, node)
	}
	sort.Strings(path.StorageNodes)

	path.TotalDelay = edgePath[len(edgePath)-1].To.Time.Sub(edgePath[0].From.Time)
	return path, nil
}

func (s *Scheduler) canStoreMessage(nodeID string, msgSize uint64) bool {
	if nodeID == "" || msgSize == 0 {
		return true
	}
	used, capacity, err := s.State.GetStorageUsage(nodeID)
	if err != nil {
		return false
	}
	if capacity == 0 {
		return false
	}
	if used+msgSize > capacity {
		return false
	}
	return true
}

type dijkstraNode struct {
	nodeIdx int
	cost    int
}

type dijkstraQueue []dijkstraNode

func (pq dijkstraQueue) Len() int            { return len(pq) }
func (pq dijkstraQueue) Less(i, j int) bool  { return pq[i].cost < pq[j].cost }
func (pq dijkstraQueue) Swap(i, j int)       { pq[i], pq[j] = pq[j], pq[i] }
func (pq *dijkstraQueue) Push(x interface{}) { *pq = append(*pq, x.(dijkstraNode)) }
func (pq *dijkstraQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	*pq = old[:n-1]
	return item
}
