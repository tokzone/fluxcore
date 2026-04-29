package fluxcore

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tokzone/fluxcore/errors"
)

func TestIsNetworkError(t *testing.T) {
	netErr := errors.Wrap(errors.CodeNetworkError, "connection refused", nil)
	dnsErr := errors.Wrap(errors.CodeDNSError, "no such host", nil)
	timeoutErr := errors.Wrap(errors.CodeTimeout, "timeout", nil)
	modelErr := errors.Wrap(errors.CodeServerError, "server error", nil)

	if !isNetworkError(netErr) {
		t.Error("CodeNetworkError should be network error")
	}
	if !isNetworkError(dnsErr) {
		t.Error("CodeDNSError should be network error")
	}
	if !isNetworkError(timeoutErr) {
		t.Error("CodeTimeout should be network error")
	}
	if isNetworkError(modelErr) {
		t.Error("CodeServerError should not be network error")
	}
}

func TestIsModelError(t *testing.T) {
	rateLimitErr := errors.Wrap(errors.CodeRateLimit, "rate limit", nil)
	serverErr := errors.Wrap(errors.CodeServerError, "500", nil)
	modelErr := errors.Wrap(errors.CodeModelError, "model overloaded", nil)
	authErr := errors.Wrap(errors.CodeAuthError, "unauthorized", nil)

	if !isModelError(rateLimitErr) {
		t.Error("CodeRateLimit should be model error")
	}
	if !isModelError(serverErr) {
		t.Error("CodeServerError should be model error")
	}
	if !isModelError(modelErr) {
		t.Error("CodeModelError should be model error")
	}
	if isModelError(authErr) {
		t.Error("CodeAuthError should not be model error")
	}
}

func TestRouter_Feedback_NetworkFailure(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	route := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})
	r := NewRouter(ProtocolOpenAI)

	netErr := errors.Wrap(errors.CodeNetworkError, "connection failed", nil)
	r.feedbackFailure(route, netErr)

	if se.IsAvailable() {
		t.Error("service endpoint should be tripped by network error")
	}
	// Route.IsAvailable also checks SvcEP, so route is unavailable due to SE.
	// But the route's own model-layer CB should not be tripped.
	if route.FailCount() != 0 {
		t.Error("route model CB should NOT be tripped by network error (only model errors)")
	}
}

func TestRouter_Feedback_ModelFailure(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	route := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})
	r := NewRouter(ProtocolOpenAI)

	modelErr := errors.Wrap(errors.CodeServerError, "500 internal server error", nil)
	r.feedbackFailure(route, modelErr)

	if !se.IsAvailable() {
		t.Error("service endpoint should NOT be tripped by model error")
	}
	if route.FailCount() != 1 {
		t.Errorf("route fail count = %d, want 1", route.FailCount())
	}
}

func TestRouter_Feedback_NonRetryableDoesNotTrip(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	route := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})
	r := NewRouter(ProtocolOpenAI)

	authErr := errors.Wrap(errors.CodeAuthError, "401 unauthorized", nil)
	r.feedbackFailure(route, authErr)

	if !se.IsAvailable() {
		t.Error("service endpoint should NOT be tripped by auth error")
	}
	if route.FailCount() != 0 {
		t.Error("route should NOT be tripped by auth error")
	}
}

func TestRouter_Feedback_Success(t *testing.T) {
	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: "https://example.com"}})
	route := NewRoute(RouteDesc{SvcEP: se, Model: "m", Credential: "k", Priority: 0})
	r := NewRouter(ProtocolOpenAI)

	// Trip first
	se.MarkNetworkFailure()
	route.MarkModelFailure()

	r.feedbackSuccess(route, 100)

	if !se.IsAvailable() {
		t.Error("service endpoint should be available after success")
	}
	if !route.IsAvailable() {
		t.Error("route should be available after success")
	}
	if l := se.LatencyEWMA(); l != 100 {
		t.Errorf("se latency = %d, want 100", l)
	}
	if l := route.LatencyEWMA(); l != 100 {
		t.Errorf("route latency = %d, want 100", l)
	}
}

func TestRouter_Execute_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"chat-1","model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()

	se := NewServiceEndpoint(Service{Name: "test", BaseURLs: map[Protocol]string{ProtocolOpenAI: srv.URL}})
	route := NewRoute(RouteDesc{SvcEP: se, Model: "gpt-4", Credential: "sk-test", Priority: 0})
	table := NewRouteTable([]*Route{route}, ProtocolOpenAI)
	r := NewRouter(ProtocolOpenAI)

	body := []byte(`{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`)
	selectedRoute, resp, usage, err := r.Execute(context.Background(), table, body, 2)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selectedRoute.IdentityKey() != route.IdentityKey() {
		t.Error("should return the selected route")
	}
	if resp == nil {
		t.Error("response should not be nil")
	}
	if usage == nil {
		t.Fatal("usage should not be nil")
	}
	if usage.InputTokens != 10 || usage.OutputTokens != 5 {
		t.Errorf("usage = %+v, want input=10 output=5", usage)
	}
}

func TestRouter_Execute_Failover(t *testing.T) {
	callCount := 0
	srv1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(500) // Simulate server error
	}))
	defer srv1.Close()

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"c2","model":"m","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv2.Close()

	se1 := NewServiceEndpoint(Service{Name: "s1", BaseURLs: map[Protocol]string{ProtocolOpenAI: srv1.URL}})
	se2 := NewServiceEndpoint(Service{Name: "s2", BaseURLs: map[Protocol]string{ProtocolOpenAI: srv2.URL}})

	r1 := NewRoute(RouteDesc{SvcEP: se1, Model: "m", Credential: "k1", Priority: 0})
	r2 := NewRoute(RouteDesc{SvcEP: se2, Model: "m", Credential: "k2", Priority: 1})

	table := NewRouteTable([]*Route{r1, r2}, ProtocolOpenAI)
	router := NewRouter(ProtocolOpenAI)

	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	// maxRetry=3 (4 total attempts): r1 needs 3 failures to trip, then failover to r2 on 4th attempt
	selectedRoute, _, _, err := router.Execute(context.Background(), table, body, 3)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if selectedRoute.IdentityKey() != r2.IdentityKey() {
		t.Error("should failover to r2")
	}
	if callCount < 2 {
		t.Error("should have called both servers")
	}

	// r1's route should be tripped after 3 failures (threshold=3)
	if r1.IsAvailable() {
		t.Error("r1 should be tripped after 3 failures")
	}
}

func TestRouter_Execute_NoRoutes(t *testing.T) {
	table := NewRouteTable([]*Route{}, ProtocolOpenAI)
	r := NewRouter(ProtocolOpenAI)

	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	_, _, _, err := r.Execute(context.Background(), table, body, 2)

	if err == nil {
		t.Error("expected error when no routes available")
	}
}
