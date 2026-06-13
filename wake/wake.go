// Package wake sends Wake-on-LAN magic packets and resolves configured
// machines to their MAC addresses. It is the shared primitive used by the
// send command, the web server, and the scheduler.
package wake

import (
	"fmt"
	"net"
	"strings"

	"github.com/trugamr/wol/config"
	"github.com/trugamr/wol/magicpacket"
)

// Waker sends a wake-on-LAN magic packet to the given MAC address. Injecting it
// (rather than calling Broadcast directly) lets callers substitute a fake in
// tests so they can be exercised without real UDP traffic.
type Waker func(mac net.HardwareAddr) error

// Broadcast is the production Waker: it broadcasts a real magic packet.
func Broadcast(mac net.HardwareAddr) error {
	return magicpacket.NewMagicPacket(mac).Broadcast()
}

// ByName returns the MAC address of the machine with the specified name.
func ByName(machines []config.Machine, name string) (net.HardwareAddr, error) {
	for _, machine := range machines {
		if strings.EqualFold(machine.Name, name) {
			mac, err := net.ParseMAC(machine.Mac)
			if err != nil {
				return nil, fmt.Errorf("failed to parse MAC address: %w", err)
			}
			return mac, nil
		}
	}

	return nil, fmt.Errorf("machine with name %q not found", name)
}
