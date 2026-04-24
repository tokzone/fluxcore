package routing

import (
	"strings"
	"testing"
)

func TestEndpointValidate(t *testing.T) {
	tests := []struct {
		name    string
		ep      *Endpoint
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid endpoint",
			ep: &Endpoint{
				Model: "gpt-4",
				Key: &Key{
					BaseURL:  "https://api.openai.com",
					APIKey:   "sk-test-key",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: false,
		},
		{
			name: "valid endpoint with localhost (allowed in library)",
			ep: &Endpoint{
				Model: "gpt-4",
				Key: &Key{
					BaseURL:  "http://localhost:8080",
					APIKey:   "test-key",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: false,
		},
		{
			name: "valid endpoint with private IP (allowed in library)",
			ep: &Endpoint{
				Model: "gpt-4",
				Key: &Key{
					BaseURL:  "https://10.0.0.1:443",
					APIKey:   "test-key",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: false,
		},
		{
			name: "missing Key",
			ep: &Endpoint{
				Key: nil,
			},
			wantErr: true,
			errMsg:  "Key is required",
		},
		{
			name: "missing BaseURL",
			ep: &Endpoint{
				Key: &Key{
					BaseURL:  "",
					APIKey:   "sk-test-key",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: true,
			errMsg:  "BaseURL is required",
		},
		{
			name: "invalid BaseURL scheme",
			ep: &Endpoint{
				Key: &Key{
					BaseURL:  "ftp://api.openai.com",
					APIKey:   "sk-test-key",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: true,
			errMsg:  "must use http or https",
		},
		{
			name: "missing APIKey",
			ep: &Endpoint{
				Key: &Key{
					BaseURL:  "https://api.openai.com",
					APIKey:   "",
					Protocol: ProtocolOpenAI,
				},
			},
			wantErr: true,
			errMsg:  "APIKey is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.ep.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing '%s', got nil", tt.errMsg)
				} else if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestIsPrivateIP(t *testing.T) {
	t.Skip("IsPrivateIP removed - SSRF protection is application-layer responsibility")
}

func TestUpdateLatency(t *testing.T) {
	t.Parallel()
	key := &Key{
		BaseURL:  "https://api.example.com",
		APIKey:   "test-key",
		Protocol: ProtocolOpenAI,
	}
	ep, err := NewEndpoint(1, key, "gpt-4", 1000)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}

	// First measurement should be stored directly
	ep.UpdateLatency(100)
	if ep.LatencyEWMA() != 100 {
		t.Errorf("LatencyEWMA() = %d, want 100", ep.LatencyEWMA())
	}

	// Second measurement: EWMA = 0.1 * 200 + 0.9 * 100 = 110
	ep.UpdateLatency(200)
	if ep.LatencyEWMA() != 110 {
		t.Errorf("LatencyEWMA() = %d, want 110", ep.LatencyEWMA())
	}

	// Third measurement: EWMA = 0.1 * 300 + 0.9 * 110 = 129
	ep.UpdateLatency(300)
	if ep.LatencyEWMA() != 129 {
		t.Errorf("LatencyEWMA() = %d, want 129", ep.LatencyEWMA())
	}
}

func TestLatencyEWMAConcurrent(t *testing.T) {
	key := &Key{
		BaseURL:  "https://api.example.com",
		APIKey:   "test-key",
		Protocol: ProtocolOpenAI,
	}
	ep, err := NewEndpoint(1, key, "gpt-4", 1000)
	if err != nil {
		t.Fatalf("NewEndpoint: %v", err)
	}

	// Concurrent updates
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				ep.UpdateLatency(j)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Just verify no panic and value is reasonable
	latency := ep.LatencyEWMA()
	if latency < 0 || latency > 1000 {
		t.Errorf("LatencyEWMA() = %d, want 0-1000", latency)
	}
}

func TestNewEndpointSuccess(t *testing.T) {
	t.Parallel()
	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, err := NewEndpoint(1, key, "", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ep.ID != 1 {
		t.Errorf("expected ID 1, got %d", ep.ID)
	}
	if ep.Priority != 100 {
		t.Errorf("expected Priority 100, got %d", ep.Priority)
	}
}

func TestNewEndpointNilKey(t *testing.T) {
	t.Parallel()
	ep, err := NewEndpoint(1, nil, "", 0)
	if ep != nil {
		t.Errorf("expected nil endpoint, got %v", ep)
	}
	if err == nil {
		t.Error("expected error for nil key")
	}
}

func TestNewEndpointWithConfig(t *testing.T) {
	t.Parallel()
	t.Skip("NewEndpointWithConfig is now internal - tested via NewEndpoint")
}

func TestEndpointSetPriority(t *testing.T) {
	t.Parallel()
	key := &Key{BaseURL: "https://api.example.com", APIKey: "key", Protocol: ProtocolOpenAI}
	ep, _ := NewEndpoint(1, key, "", 100)
	ep.SetPriority(200)
	if ep.Priority != 200 {
		t.Errorf("expected Priority 200, got %d", ep.Priority)
	}
}