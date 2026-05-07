package nats

import (
	"strings"
	"testing"
)

// TestNewEmbedded_PortRange covers the wrapper-level validation that
// the port is in the legal TCP range (1..65535). Mirrors the same
// fail-fast pattern used by embedded postgres — better to surface a
// clear error here than to let the upstream library crash mid-listen.
func TestNewEmbedded_PortRange(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero", 0},
		{"negative", -1},
		{"above max", 65536},
		{"way above max", 1_000_000},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEmbedded(EmbeddedConfig{
				Port:    tc.port,
				DataDir: t.TempDir(),
			})
			if err == nil {
				t.Fatalf("NewEmbedded(port=%d) expected error, got nil", tc.port)
			}
			if !strings.Contains(err.Error(), "port") {
				t.Errorf("error %q should mention 'port'", err.Error())
			}
		})
	}
}

// TestNewEmbedded_MonitorPortRange covers the same fail-fast for the
// optional monitoring port. 0 is valid (means "disabled"); anything
// else must still be a legal TCP port.
func TestNewEmbedded_MonitorPortRange(t *testing.T) {
	tests := []struct {
		name        string
		monitorPort int
		wantErr     bool
	}{
		{"disabled (0)", 0, false},
		{"negative", -1, true},
		{"above max", 65536, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewEmbedded(EmbeddedConfig{
				Port:        4222,
				MonitorPort: tc.monitorPort,
				DataDir:     t.TempDir(),
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("NewEmbedded(monitor=%d) expected error, got nil", tc.monitorPort)
				}
				if !strings.Contains(err.Error(), "monitor port") {
					t.Errorf("error %q should mention 'monitor port'", err.Error())
				}
				return
			}
			// MonitorPort == 0 should not fail validation. The
			// constructor still proceeds to instantiate an upstream
			// server.Server — that does NOT bind a listener, so it
			// is safe to construct (and discard) without Start().
			if err != nil {
				t.Fatalf("NewEmbedded(monitor=0) unexpected error: %v", err)
			}
		})
	}
}
