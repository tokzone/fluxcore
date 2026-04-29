package fluxcore

import (
	"math/rand"
	"slices"
)

// tableEntry is a pre-computed route entry within a RouteTable.
type tableEntry struct {
	route       *Route
	targetProto Protocol
	baseURL     string
}

// RouteTable is an immutable value object representing a materialized view of routes
// for a specific input protocol. It pre-computes target protocols and base URLs,
// and sorts entries by priority (equal priorities are randomly shuffled).
type RouteTable struct {
	entries []tableEntry
}

// NewRouteTable creates a new RouteTable from the given routes and input protocol.
// The table is fully pre-computed and immutable after construction.
func NewRouteTable(routes []*Route, inputProto Protocol) *RouteTable {
	entries := make([]tableEntry, 0, len(routes))
	for _, r := range routes {
		svc := r.SvcEP().Service()
		targetProto := selectTargetProto(svc, inputProto)
		entries = append(entries, tableEntry{
			route:       r,
			targetProto: targetProto,
			baseURL:     svc.BaseURLs[targetProto],
		})
	}

	// Sort by priority deterministically, then shuffle equal-priority groups.
	slices.SortFunc(entries, func(a, b tableEntry) int {
		pa := a.route.Desc().Priority
		pb := b.route.Desc().Priority
		if pa < pb {
			return -1
		}
		if pa > pb {
			return 1
		}
		return 0
	})

	for i := 0; i < len(entries); {
		j := i + 1
		for j < len(entries) && entries[j].route.Desc().Priority == entries[i].route.Desc().Priority {
			j++
		}
		if j-i > 1 {
			rand.Shuffle(j-i, func(a, b int) {
				entries[i+a], entries[i+b] = entries[i+b], entries[i+a]
			})
		}
		i = j
	}

	return &RouteTable{entries: entries}
}

// Select returns the first available Route and its target protocol.
// Returns nil and zero Protocol if no route is available.
func (rt *RouteTable) Select() (*Route, Protocol) {
	for _, e := range rt.entries {
		if e.route.IsAvailable() {
			return e.route, e.targetProto
		}
	}
	return nil, 0
}

// Len returns the number of entries in the table.
func (rt *RouteTable) Len() int {
	return len(rt.entries)
}

// Routes returns all routes in the table (in priority order).
func (rt *RouteTable) Routes() []*Route {
	routes := make([]*Route, len(rt.entries))
	for i, e := range rt.entries {
		routes[i] = e.route
	}
	return routes
}

// selectTargetProto determines the target protocol for a given service and input protocol.
// If the service supports the input protocol, use it (passthrough).
// Otherwise, return the first protocol in the priority order that the service supports.
func selectTargetProto(svc Service, inputProto Protocol) Protocol {
	if _, ok := svc.BaseURLs[inputProto]; ok {
		return inputProto
	}
	for _, p := range ProtocolPriority() {
		if _, ok := svc.BaseURLs[p]; ok {
			return p
		}
	}
	return ProtocolOpenAI
}
