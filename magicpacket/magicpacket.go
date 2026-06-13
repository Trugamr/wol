package magicpacket

import (
	"io"
	"net"
)

const (
	// A magic packet is a 6-byte 0xFF sync stream followed by the MAC repeated 16 times.
	packetSize     = 102
	syncStreamSize = 6
	macRepeatCount = 16

	broadcastPort = 9
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

// Broadcast sends the magic packet to the broadcast address
func (p *MagicPacket) Broadcast() error {
	// TODO: broadcast to more common ports and addresses?
	addr := &net.UDPAddr{
		IP:   net.IPv4bcast,
		Port: broadcastPort,
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	_, err = p.WriteTo(conn)
	return err
}
