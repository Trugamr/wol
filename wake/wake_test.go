package wake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trugamr/wol/config"
)

func TestByName(t *testing.T) {
	machines := []config.Machine{
		{Name: "alpha", Mac: "01:02:03:04:05:06"},
		{Name: "badmac", Mac: "not-a-mac"},
	}

	tests := []struct {
		name      string
		lookup    string
		wantMAC   string
		wantError bool
	}{
		{name: "exact match", lookup: "alpha", wantMAC: "01:02:03:04:05:06"},
		{name: "case-insensitive match", lookup: "ALPHA", wantMAC: "01:02:03:04:05:06"},
		{name: "not found", lookup: "ghost", wantError: true},
		{name: "invalid mac", lookup: "badmac", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mac, err := ByName(machines, tt.lookup)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMAC, mac.String())
		})
	}
}
