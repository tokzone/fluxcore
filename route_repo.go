package fluxcore

import (
	"sync"
	"time"
)

// RouteRepository caches Route aggregates by identity key for cross-request
// and cross-Reload circuit breaker state preservation.
//
// TTL of 300s exceeds the maximum CB recovery timeout (120s), ensuring CB state
// is always preserved while entries are alive. Entries are evicted lazily on
// access and via periodic background cleanup.
type RouteRepository struct {
	mu     sync.RWMutex
	routes map[string]*repoEntry
	done   chan struct{}
}

type repoEntry struct {
	route     *Route
	createdAt time.Time
}

const (
	routeRepoTTL     = 300 * time.Second
	routeRepoCleanup = 120 * time.Second
	routeRepoMax     = 50000
)

// NewRouteRepository creates a new RouteRepository and starts background cleanup.
func NewRouteRepository() *RouteRepository {
	r := &RouteRepository{
		routes: make(map[string]*repoEntry),
		done:   make(chan struct{}),
	}
	go r.cleanupLoop()
	return r
}

// FindOrCreate returns an existing Route if present and not expired, otherwise
// calls create() to build a new one and stores it under the given key.
func (r *RouteRepository) FindOrCreate(key string, create func() *Route) *Route {
	r.mu.RLock()
	if entry, ok := r.routes[key]; ok && time.Since(entry.createdAt) < routeRepoTTL {
		route := entry.route
		r.mu.RUnlock()
		return route
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.routes[key]; ok && time.Since(entry.createdAt) < routeRepoTTL {
		return entry.route
	}

	// Evict oldest if at capacity
	if len(r.routes) >= routeRepoMax {
		r.evictOldestLocked()
	}

	route := create()
	r.routes[key] = &repoEntry{route: route, createdAt: time.Now()}
	return route
}

// All returns all non-expired cached Route aggregates.
func (r *RouteRepository) All() []*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	var result []*Route
	for k, e := range r.routes {
		if now.Sub(e.createdAt) < routeRepoTTL {
			result = append(result, e.route)
		} else {
			delete(r.routes, k)
		}
	}
	return result
}

// RoutesByServiceEndpoint groups all cached Routes by their ServiceEndpoint name.
func (r *RouteRepository) RoutesByServiceEndpoint() map[string][]*Route {
	r.mu.RLock()
	defer r.mu.RUnlock()
	now := time.Now()
	result := make(map[string][]*Route)
	for _, e := range r.routes {
		if now.Sub(e.createdAt) >= routeRepoTTL {
			continue
		}
		name := e.route.SvcEP().Service().Name
		result[name] = append(result[name], e.route)
	}
	return result
}

// Close stops the background cleanup goroutine.
func (r *RouteRepository) Close() {
	close(r.done)
}

func (r *RouteRepository) evictOldestLocked() {
	var oldestKey string
	var oldestTime time.Time
	for k, e := range r.routes {
		if oldestKey == "" || e.createdAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.createdAt
		}
	}
	if oldestKey != "" {
		delete(r.routes, oldestKey)
	}
}

func (r *RouteRepository) cleanupLoop() {
	ticker := time.NewTicker(routeRepoCleanup)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			r.mu.Lock()
			now := time.Now()
			for k, e := range r.routes {
				if now.Sub(e.createdAt) > routeRepoTTL {
					delete(r.routes, k)
				}
			}
			r.mu.Unlock()
		}
	}
}
