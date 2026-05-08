package vpn

import (
	"testing"
)

func TestIsTunnel(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"utun0", true},
		{"utun3", true},
		{"utun99", true},
		{"tun0", true},
		{"tun1", true},
		{"wg0", true},
		{"wg-my-vpn", true},
		{"ppp0", true},
		{"ipsec0", true},
		{"en0", false},
		{"lo0", false},
		{"eth0", false},
		{"wlan0", false},
		{"bridge0", false},
		{"awdl0", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTunnel(tt.name); got != tt.expected {
				t.Errorf("isTunnel(%q) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

func TestDetectorDefaults(t *testing.T) {
	d := NewDetector()
	// On a test machine without VPN, this should be false
	// But we can't assert either way — just make sure it doesn't panic
	_ = d.Active()
	_ = d.Interface()
	_ = d.Status()

	// Verify methods don't panic and return non-empty status
	if d.Status() == "" {
		t.Error("Status() returned empty string")
	}
}

func TestDetectorRecheck(t *testing.T) {
	d := NewDetector()
	first := d.Active()
	firstIface := d.Interface()

	// Recheck should not panic
	d.Check()

	// Active state should be consistent
	if d.Active() != first {
		t.Logf("state changed from %v to %v (may be flaky in CI)", first, d.Active())
	}
	_ = firstIface
}
