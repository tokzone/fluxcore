package flux

import (
	"errors"
	"fmt"

	"github.com/tokzone/fluxcore/endpoint"
	"github.com/tokzone/fluxcore/provider"
)

// APIKey represents a user's API key for a Provider.
// Multiple UserEndpoints can share the same APIKey.
type APIKey struct {
	provider *provider.Provider // Pointer to global Provider singleton (no memory overhead)
	secret   string             // Secret
}

var (
	errNilAPIKeyProvider = errors.New("apikey provider cannot be nil")
	errEmptyAPIKeySecret = errors.New("apikey secret cannot be empty")
)

// NewAPIKey creates a new APIKey for a Provider.
func NewAPIKey(prov *provider.Provider, secret string) (*APIKey, error) {
	if prov == nil {
		return nil, errNilAPIKeyProvider
	}
	if secret == "" {
		return nil, errEmptyAPIKeySecret
	}
	return &APIKey{provider: prov, secret: secret}, nil
}

func (k *APIKey) Provider() *provider.Provider {
	return k.provider
}

func (k *APIKey) Secret() string {
	return k.secret
}

// UserEndpoint represents a user's request endpoint configuration.
// UserEndpoint combines an Endpoint (Provider+Model) with an APIKey and Priority.
type UserEndpoint struct {
	endpoint *endpoint.Endpoint // Cached from registry at creation
	apiKey   *APIKey            // Reference to user's APIKey
	priority int64              // User's private priority (lower = preferred)
}

var errNilUserEndpointAPIKey = errors.New("userendpoint apikey cannot be nil")

// NewUserEndpoint creates a new UserEndpoint.
// The model parameter is required for Gemini (used in URL construction).
// For OpenAI, Anthropic, and Cohere, pass empty string "".
// Endpoint must be registered before creating UserEndpoint.
func NewUserEndpoint(model string, apiKey *APIKey, priority int64) (*UserEndpoint, error) {
	if apiKey == nil {
		return nil, errNilUserEndpointAPIKey
	}

	prov := apiKey.Provider()
	ep := endpoint.GlobalRegistry().GetByProviderModel(prov, model)
	if ep == nil {
		return nil, fmt.Errorf("endpoint not registered: provider=%d, model=%q", prov.ID, model)
	}

	return &UserEndpoint{
		endpoint: ep,
		apiKey:   apiKey,
		priority: priority,
	}, nil
}

func (ue *UserEndpoint) Endpoint() *endpoint.Endpoint {
	return ue.endpoint
}

func (ue *UserEndpoint) APIKey() *APIKey {
	return ue.apiKey
}

func (ue *UserEndpoint) Priority() int64 {
	return ue.priority
}

func (ue *UserEndpoint) Provider() *provider.Provider {
	return ue.endpoint.Provider()
}

func (ue *UserEndpoint) Model() string {
	return ue.endpoint.Model
}

func (ue *UserEndpoint) Secret() string {
	return ue.apiKey.secret
}

func (ue *UserEndpoint) Protocol() provider.Protocol {
	return ue.endpoint.Protocol()
}

// SelectProtocol returns the matching protocol if the endpoint supports it, otherwise the default.
func (ue *UserEndpoint) SelectProtocol(input provider.Protocol) provider.Protocol {
	return ue.endpoint.SelectProtocol(input)
}

func (ue *UserEndpoint) BaseURL(proto provider.Protocol) string {
	return ue.endpoint.BaseURL(proto)
}
