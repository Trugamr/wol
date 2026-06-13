package magicpacket

import (
	"fmt"
	"io"
	"net"
)

const (
	// A magic packet is a 6-byte 0xFF sync stream followed by the MAC repeated 16 times.
	packetSize     = 102
	syncStreamSize = 6
	macRepeatCount = 16
)

// MagicPacket represents a wake-on-LAN packet
type MagicPacket struct {
	// The MAC address of the machine to wake up
	MacAddress net.HardwareAddr
}

// NewMagicPacket creates a new MagicPacket for the given MAC address
func NewMagicPacket(macAddress net.HardwareAddr) *MagicPacket {
	return &MagicPacket{MacAddress: macAddress}
}

// Bytes returns the raw magic packet payload.
func (p *MagicPacket) Bytes() []byte {
	packet := make([]byte, packetSize)
	for i := 0; i < syncStreamSize; i++ {
		packet[i] = 0xFF
	}
	for i := 1; i <= macRepeatCount; i++ {
		copy(packet[i*6:], p.MacAddress)
	}
	return packet
}

// WriteTo writes the packet to w. It implements io.WriterTo so the send path can
// be tested against an in-memory writer instead of the network.
func (p *MagicPacket) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(p.Bytes())
	return int64(n), err
}

// Broadcast sends the magic packet to addr, a "host:port" broadcast target such
// as "255.255.255.255:9" or a subnet-directed broadcast like "192.168.1.255:9".
// A subnet-directed broadcast is routed out the interface owning that subnet,
// which is what makes waking devices reliable on multi-homed hosts.
func (p *MagicPacket) Broadcast(addr string) (err error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("invalid broadcast address %q: %w", addr, err)
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	// Surface a close error, but don't let it mask a write error.
	defer func() {
		if cerr := conn.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	_, err = p.WriteTo(conn)
	return err
}
