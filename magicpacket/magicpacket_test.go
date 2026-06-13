package magicpacket

import (
	"bytes"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMagicPacketBytes(t *testing.T) {
	tests := []struct {
		name string
		mac  string
	}{
		{name: "typical mac", mac: "01:02:03:04:05:06"},
		{name: "all ones mac", mac: "ff:ff:ff:ff:ff:ff"},
		{name: "zero mac", mac: "00:00:00:00:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac, err := net.ParseMAC(tt.mac)
			require.NoError(t, err)

			packet := NewMagicPacket(mac).Bytes()

			require.Len(t, packet, 102)
			assert.Equal(t, bytes.Repeat([]byte{0xFF}, 6), packet[:6])

			// MAC repeated 16 times after the sync stream.
			for i := 0; i < 16; i++ {
				start := 6 + i*6
				assert.Equalf(t, []byte(mac), packet[start:start+6],
					"MAC repetition %d at offset %d", i+1, start)
			}
		})
	}
}

func TestMagicPacketWriteTo(t *testing.T) {
	mac, err := net.ParseMAC("01:02:03:04:05:06")
	require.NoError(t, err)

	mp := NewMagicPacket(mac)

	var buf bytes.Buffer
	n, err := mp.WriteTo(&buf)
	require.NoError(t, err)

	assert.Equal(t, int64(102), n)
	assert.Equal(t, mp.Bytes(), buf.Bytes())
}
