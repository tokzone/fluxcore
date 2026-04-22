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
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"public domain", "api.openai.com", false},
		{"public IP", "1.2.3.4", false},
		{"localhost", "localhost", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"0.0.0.0", "0.0.0.0", true},
		{"::1", "::1", true},
		{"10.x private", "10.0.0.1", true},
		{"10.x private 2", "10.255.255.255", true},
		{"172.16.x private", "172.16.0.1", true},
		{"172.31.x private", "172.31.255.255", true},
		{"172.15.x public", "172.15.0.1", false},
		{"172.32.x public", "172.32.0.1", false},
		{"192.168.x private", "192.168.1.1", true},
		{"192.169.x public", "192.169.1.1", false},
		{"169.254.x cloud metadata", "169.254.169.254", true},
		{"169.254.x link-local", "169.254.1.1", true},
		{"fe80:: IPv6 link-local", "fe80::1", true},
		{"2001:: IPv6 public", "2001::1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPrivateIP(tt.host)
			if result != tt.expected {
				t.Errorf("IsPrivateIP(%s) = %v, expected %v", tt.host, result, tt.expected)
			}
		})
	}
}