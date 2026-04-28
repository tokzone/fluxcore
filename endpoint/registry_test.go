package endpoint

import (
	"testing"

	"github.com/tokzone/fluxcore/provider"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected registry")
	}
}

func TestGlobalRegistry(t *testing.T) {
	r := GlobalRegistry()
	if r == nil {
		t.Fatal("expected global registry")
	}
}

func TestRegistryRegister(t *testing.T) {
	r := NewRegistry()
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep, _ := NewEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})

	r.Register(ep)

	if r.Get(1) != ep {
		t.Error("expected to find registered endpoint")
	}
}

func TestRegistryGetByProviderModel(t *testing.T) {
	r := NewRegistry()
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep, _ := NewEndpoint(1, prov, "gpt-4", []provider.Protocol{provider.ProtocolOpenAI})

	r.Register(ep)

	found := r.GetByProviderModel(prov, "gpt-4")
	if found != ep {
		t.Error("expected to find endpoint by provider+model")
	}

	// Not found
	notFound := r.GetByProviderModel(prov, "gpt-3.5")
	if notFound != nil {
		t.Error("expected nil for non-existent model")
	}
}

func TestRegistryGetAll(t *testing.T) {
	r := NewRegistry()
	prov1 := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	prov2 := provider.NewProvider(2, provider.SingleBaseURL("https://api.anthropic.com"))

	ep1, _ := NewEndpoint(1, prov1, "", []provider.Protocol{provider.ProtocolOpenAI})
	ep2, _ := NewEndpoint(2, prov2, "", []provider.Protocol{provider.ProtocolAnthropic})

	r.Register(ep1)
	r.Register(ep2)

	all := r.GetAll()
	if len(all) != 2 {
		t.Errorf("expected 2 endpoints, got %d", len(all))
	}
}

func TestRegistryClear(t *testing.T) {
	r := NewRegistry()
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	ep, _ := NewEndpoint(1, prov, "", []provider.Protocol{provider.ProtocolOpenAI})

	r.Register(ep)
	r.Clear()

	if r.Get(1) != nil {
		t.Error("expected nil after clear")
	}
}

func TestRegisterEndpoint(t *testing.T) {
	// RegisterEndpoint uses globalRegistry
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))

	ep := RegisterEndpoint(2, prov, "", []provider.Protocol{provider.ProtocolOpenAI})
	if ep == nil {
		t.Fatal("expected endpoint from RegisterEndpoint")
	}

	// Verify it's in global registry
	if GlobalRegistry().Get(2) != ep {
		t.Error("expected endpoint in global registry")
	}

	// Clean up global registry
	GlobalRegistry().Clear()
}

func TestRegistryMultipleProviders(t *testing.T) {
	r := NewRegistry()

	prov1 := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))
	prov2 := provider.NewProvider(2, provider.SingleBaseURL("https://api.anthropic.com"))

	ep1, _ := NewEndpoint(1, prov1, "", []provider.Protocol{provider.ProtocolOpenAI})
	ep2, _ := NewEndpoint(2, prov2, "", []provider.Protocol{provider.ProtocolAnthropic})

	r.Register(ep1)
	r.Register(ep2)

	// Find by different providers
	if r.GetByProviderModel(prov1, "") != ep1 {
		t.Error("expected ep1 for prov1")
	}
	if r.GetByProviderModel(prov2, "") != ep2 {
		t.Error("expected ep2 for prov2")
	}
}

func TestRegistrySameProviderDifferentModels(t *testing.T) {
	r := NewRegistry()
	prov := provider.NewProvider(1, provider.SingleBaseURL("https://api.openai.com"))

	ep1, _ := NewEndpoint(1, prov, "gpt-4", []provider.Protocol{provider.ProtocolOpenAI})
	ep2, _ := NewEndpoint(2, prov, "gpt-3.5", []provider.Protocol{provider.ProtocolOpenAI})

	r.Register(ep1)
	r.Register(ep2)

	if r.GetByProviderModel(prov, "gpt-4") != ep1 {
		t.Error("expected ep1 for gpt-4")
	}
	if r.GetByProviderModel(prov, "gpt-3.5") != ep2 {
		t.Error("expected ep2 for gpt-3.5")
	}
}
