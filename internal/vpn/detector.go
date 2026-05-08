package vpn

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Detector checks whether a VPN tunnel interface is active.
type Detector struct {
	mu       sync.RWMutex
	active   bool
	iface    string // name of the detected VPN interface
	lastCheck time.Time
}

// NewDetector creates a VPN detector and runs an initial check.
func NewDetector() *Detector {
	d := &Detector{}
	d.Check()
	return d
}

// Check scans network interfaces for active VPN tunnels.
func (d *Detector) Check() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.lastCheck = time.Now()

	ifaces, err := net.Interfaces()
	if err != nil {
		d.active = false
		d.iface = ""
		return
	}

	for _, iface := range ifaces {
		// Look for tunnel interfaces that are up
		if !isTunnel(iface.Name) {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Must have an IP address assigned (not just the interface existing)
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		hasIP := false
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP != nil && !ipnet.IP.IsLoopback() && !ipnet.IP.IsLinkLocalUnicast() {
					hasIP = true
					break
				}
			}
		}
		if !hasIP {
			continue
		}

		d.active = true
		d.iface = iface.Name
		return
	}

	d.active = false
	d.iface = ""
}

// Active returns whether a VPN tunnel is currently detected.
func (d *Detector) Active() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.active
}

// Interface returns the name of the detected VPN interface (e.g. "utun3").
func (d *Detector) Interface() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.iface
}

// Status returns a human-readable status line.
func (d *Detector) Status() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.active {
		return fmt.Sprintf("🔒 VPN active (%s)", d.iface)
	}
	return "🔓 No VPN detected"
}

// isTunnel returns true if the interface name looks like a VPN tunnel.
func isTunnel(name string) bool {
	// macOS: utun0, utun1, etc.
	if strings.HasPrefix(name, "utun") {
		return true
	}
	// Generic tunnel interfaces (Linux, some macOS configs)
	if name == "tun0" || strings.HasPrefix(name, "tun") {
		return true
	}
	// WireGuard specific
	if name == "wg0" || strings.HasPrefix(name, "wg") {
		return true
	}
	// PPP (some older VPNs)
	if strings.HasPrefix(name, "ppp") {
		return true
	}
	// IPsec
	if strings.HasPrefix(name, "ipsec") {
		return true
	}
	return false
}
