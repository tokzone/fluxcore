package fluxcore

import (
	"testing"
)

func makeTestRoute(name, model, cred string, priority int64) *Route {
	se := NewServiceEndpoint(Service{Name: name, BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://" + name + ".com/v1"}})
	return NewRoute(RouteDesc{SvcEP: se, Model: Model(model), Credential: cred, Priority: priority})
}

func TestRouteTable_Select_FirstAvailable(t *testing.T) {
	r1 := makeTestRoute("openai", "gpt-4", "sk-a", 0)
	r2 := makeTestRoute("anthropic", "claude-3", "sk-b", 1)

	table := NewRouteTable([]*Route{r1, r2}, ProtocolOpenAI)

	route, proto := table.Select()
	if route == nil {
		t.Fatal("expected a route")
	}
	if route.IdentityKey() != r1.IdentityKey() {
		t.Error("should select highest priority (lowest number) route first")
	}
	if proto != ProtocolOpenAI {
		t.Errorf("targetProto = %v, want openai", proto)
	}
}

func TestRouteTable_Select_SkipsUnavailable(t *testing.T) {
	r1 := makeTestRoute("openai", "gpt-4", "sk-a", 0)
	r2 := makeTestRoute("anthropic", "claude-3", "sk-b", 1)

	r1.MarkModelFailure()
	r1.MarkModelFailure()
	r1.MarkModelFailure()

	table := NewRouteTable([]*Route{r1, r2}, ProtocolOpenAI)

	route, _ := table.Select()
	if route == nil {
		t.Fatal("expected a route")
	}
	if route.IdentityKey() != r2.IdentityKey() {
		t.Error("should skip unavailable route and select next")
	}
}

func TestRouteTable_Select_AllUnavailable(t *testing.T) {
	r1 := makeTestRoute("openai", "gpt-4", "sk-a", 0)

	r1.SvcEP().MarkNetworkFailure()

	table := NewRouteTable([]*Route{r1}, ProtocolOpenAI)

	route, _ := table.Select()
	if route != nil {
		t.Error("should return nil when all routes unavailable")
	}
}

func TestRouteTable_Select_PassthroughProtocol(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "anthropic", BaseURLs: map[Protocol]string{
		ProtocolAnthropic: "https://api.anthropic.com/v1",
	}})
	r := NewRoute(RouteDesc{SvcEP: se, Model: "claude-3", Credential: "sk-key", Priority: 0})

	// Input is Anthropic, service supports Anthropic → passthrough
	table := NewRouteTable([]*Route{r}, ProtocolAnthropic)
	_, proto := table.Select()
	if proto != ProtocolAnthropic {
		t.Errorf("should passthrough, got %v", proto)
	}

	// Input is OpenAI, service only supports Anthropic → use Anthropic
	table2 := NewRouteTable([]*Route{r}, ProtocolOpenAI)
	_, proto2 := table2.Select()
	if proto2 != ProtocolAnthropic {
		t.Errorf("should fallback to available protocol, got %v", proto2)
	}
}

func TestRouteTable_Len(t *testing.T) {
	r1 := makeTestRoute("openai", "gpt-4", "sk-a", 0)
	r2 := makeTestRoute("anthropic", "claude-3", "sk-b", 1)

	table := NewRouteTable([]*Route{r1, r2}, ProtocolOpenAI)
	if table.Len() != 2 {
		t.Errorf("Len = %d, want 2", table.Len())
	}
}

func TestRouteTable_Routes(t *testing.T) {
	r1 := makeTestRoute("openai", "gpt-4", "sk-a", 0)
	r2 := makeTestRoute("anthropic", "claude-3", "sk-b", 1)

	table := NewRouteTable([]*Route{r1, r2}, ProtocolOpenAI)
	routes := table.Routes()

	if len(routes) != 2 {
		t.Fatalf("len = %d, want 2", len(routes))
	}
}

func TestRouteTable_SortByPriority(t *testing.T) {
	rLow := makeTestRoute("a", "m1", "k1", 10)
	rHigh := makeTestRoute("b", "m2", "k2", 0)

	table := NewRouteTable([]*Route{rLow, rHigh}, ProtocolOpenAI)
	route, _ := table.Select()

	if route.Desc().Priority != 0 {
		t.Errorf("should select lowest priority first, got priority=%d", route.Desc().Priority)
	}
}
