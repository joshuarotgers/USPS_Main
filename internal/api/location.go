package api

import (
	"sync"
)

// LatestLocation holds the latest known location for a driver on a route.
type LatestLocation struct {
	Tenant   string  `json:"tenantId"`
	RouteID  string  `json:"routeId"`
	DriverID string  `json:"driverId"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	TS       string  `json:"ts"`
}

// LocationCache stores latest driver locations per tenant/route/driver.
type LocationCache struct {
	mu sync.Mutex
	// key: tenant|routeId|driverId
	m map[string]LatestLocation
}

// NewLocationCache constructs a LocationCache.
func NewLocationCache() *LocationCache { return &LocationCache{m: map[string]LatestLocation{}} }

func (c *LocationCache) key(tenant, routeID, driverID string) string {
	return tenant + "|" + routeID + "|" + driverID
}

// Upsert stores or updates the latest location for a driver.
func (c *LocationCache) Upsert(tenant, routeID, driverID string, lat, lng float64, ts string) {
	if tenant == "" || routeID == "" || driverID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	k := c.key(tenant, routeID, driverID)
	c.m[k] = LatestLocation{Tenant: tenant, RouteID: routeID, DriverID: driverID, Lat: lat, Lng: lng, TS: ts}
}

// ListByRoute returns the latest locations for drivers on a route.
func (c *LocationCache) ListByRoute(tenant, routeID string) []LatestLocation {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := []LatestLocation{}
	prefix := tenant + "|" + routeID + "|"
	for k, v := range c.m {
		// simple prefix match
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, v)
		}
	}
	return out
}
