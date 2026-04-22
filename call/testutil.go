package call

import (
	"github.com/tokzone/fluxcore/routing"
)

// testEndpoint creates an endpoint for testing
func testEndpoint(id uint, baseURL, apiKey string, protocol routing.Protocol) *routing.Endpoint {
	key := &routing.Key{BaseURL: baseURL, APIKey: apiKey, Protocol: protocol}
	return routing.NewEndpoint(id, key, "", 0, 0)
}

// testEndpointWithPrice creates an endpoint with pricing for testing
func testEndpointWithPrice(id uint, baseURL, apiKey string, protocol routing.Protocol, inputPrice float64) *routing.Endpoint {
	key := &routing.Key{BaseURL: baseURL, APIKey: apiKey, Protocol: protocol}
	return routing.NewEndpoint(id, key, "", inputPrice, 0)
}