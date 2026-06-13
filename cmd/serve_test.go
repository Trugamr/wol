package cmd

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/trugamr/wol/config"
)

func strptr(s string) *string { return &s }

// fakePinger is a test Pinger that returns canned reachability per address.
type fakePinger struct {
	reachable map[string]bool
}

func (f fakePinger) Reachable(addr string) (bool, error) {
	return f.reachable[addr], nil
}

func TestHandleIndexRendersMachines(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{{Name: "alpha", Mac: "01:02:03:04:05:06"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	newServer(cfg, nil, nil).routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "alpha")
}

func TestHandleWakeSuccess(t *testing.T) {
	const macStr = "01:02:03:04:05:06"
	cfg := &config.Config{
		Machines: []config.Machine{{Name: "alpha", Mac: macStr}},
	}

	var gotMac net.HardwareAddr
	wake := func(mac net.HardwareAddr) error {
		gotMac = mac
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/wake", strings.NewReader("name=alpha"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	newServer(cfg, nil, wake).routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/", rec.Header().Get("Location"))

	// The configured machine's MAC was parsed and handed to the wake seam.
	want, err := net.ParseMAC(macStr)
	require.NoError(t, err)
	assert.Equal(t, want, gotMac)

	// A flash message cookie naming the machine is set.
	var flash *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "flash" {
			flash = c
		}
	}
	require.NotNil(t, flash)
	assert.Contains(t, flash.Value, "alpha")
}

func TestHandleWakeMachineNotFound(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{{Name: "alpha", Mac: "01:02:03:04:05:06"}},
	}

	called := false
	wake := func(mac net.HardwareAddr) error {
		called = true
		return nil
	}
	req := httptest.NewRequest(http.MethodPost, "/wake", strings.NewReader("name=ghost"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	newServer(cfg, nil, wake).routes().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, called, "wake must not be called for an unknown machine")
}

func TestWriteMachinesStatusFrame(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{
			{Name: "on", IP: strptr("10.0.0.1")},
			{Name: "off", IP: strptr("10.0.0.2")},
			{Name: "unknown", IP: nil},
		},
	}
	pinger := fakePinger{reachable: map[string]bool{
		"10.0.0.1": true,
		"10.0.0.2": false,
	}}
	var buf bytes.Buffer
	require.NoError(t, newServer(cfg, pinger, nil).writeMachinesStatus(&buf))

	out := buf.String()
	require.True(t, strings.HasPrefix(out, "data: "), "frame must start with an SSE data field")
	require.True(t, strings.HasSuffix(out, "\n\n"), "frame must end with a blank line")

	payload := strings.TrimSuffix(strings.TrimPrefix(out, "data: "), "\n\n")
	var statuses map[string]string
	require.NoError(t, json.Unmarshal([]byte(payload), &statuses))
	assert.Equal(t, map[string]string{
		"on":      "online",
		"off":     "offline",
		"unknown": "unknown",
	}, statuses)
}

func TestGetMacByName(t *testing.T) {
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
			mac, err := getMacByName(machines, tt.lookup)
			if tt.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantMAC, mac.String())
		})
	}
}
