package cmd

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestHandleIndexShowsScheduleChip(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{
			{Name: "scheduled", Mac: "01:02:03:04:05:06"},
			{Name: "plain", Mac: "0a:0b:0c:0d:0e:0f"},
		},
		Schedules: []config.Schedule{{Machine: "scheduled", Cron: "0 2 * * 6"}},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	newServer(cfg, nil, nil).routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	// The scheduled machine renders a chip with its cron expression...
	assert.Contains(t, body, "0 2 * * 6")
	// ...and there is exactly one chip (the unscheduled machine has none, and a
	// single schedule produces no "+N" overflow pill). Match the rendered class
	// attribute, not the bare name, which also appears in the <style> block.
	assert.Equal(t, 1, strings.Count(body, `class="machine__schedule"`))
	assert.NotContains(t, body, `class="machine__schedule machine__schedule--more"`)
}

func TestHandleIndexCollapsesExtraSchedules(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{{Name: "media-server", Mac: "01:02:03:04:05:06"}},
		Schedules: []config.Schedule{
			{Machine: "media-server", Cron: "@daily"},
			{Machine: "media-server", Cron: "@every 1m"},
			{Machine: "media-server", Cron: "0 18 * * 1-5"},
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	newServer(cfg, nil, nil).routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	// Only the first schedule is shown as a visible chip; the rest collapse into
	// a "+2" overflow pill (their crons live in its title for hover).
	assert.Equal(t, 1, strings.Count(body, `class="machine__schedule"`))
	assert.Contains(t, body, `class="machine__schedule machine__schedule--more"`)
	assert.Contains(t, body, "@daily")
	assert.Contains(t, body, ">+2<")
}

func TestHandleWakeSuccess(t *testing.T) {
	const macStr = "01:02:03:04:05:06"
	cfg := &config.Config{
		// The machine overrides only the broadcast address, so the port should be
		// inherited from the global default.
		Broadcast: config.Broadcast{Address: "255.255.255.255", Port: 9},
		Machines: []config.Machine{{
			Name:      "alpha",
			Mac:       macStr,
			Broadcast: &config.Broadcast{Address: "192.168.1.255"},
		}},
	}

	var (
		gotMac  net.HardwareAddr
		gotAddr string
	)
	wake := func(mac net.HardwareAddr, addr string) error {
		gotMac = mac
		gotAddr = addr
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

	// The per-machine broadcast override was resolved (address overridden, port
	// inherited from the global default) and passed to the wake seam.
	assert.Equal(t, "192.168.1.255:9", gotAddr)

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

// Non-ASCII machine names must survive the flash cookie round-trip (issue #35).
func TestHandleWakeNonASCIIName(t *testing.T) {
	const machineName = "客厅电脑"
	cfg := &config.Config{
		Broadcast: config.Broadcast{Address: "255.255.255.255", Port: 9},
		Machines:  []config.Machine{{Name: machineName, Mac: "01:02:03:04:05:06"}},
	}

	wake := func(mac net.HardwareAddr, addr string) error { return nil }
	body := "name=" + url.QueryEscape(machineName)
	req := httptest.NewRequest(http.MethodPost, "/wake", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	newServer(cfg, nil, wake).routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusSeeOther, rec.Code)

	var flash *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "flash" {
			flash = c
		}
	}
	require.NotNil(t, flash)

	// Decoded value must preserve the non-ASCII name (no bytes dropped).
	got, err := url.QueryUnescape(flash.Value)
	require.NoError(t, err)
	assert.Contains(t, got, machineName)
}

func TestHandleWakeMachineNotFound(t *testing.T) {
	cfg := &config.Config{
		Machines: []config.Machine{{Name: "alpha", Mac: "01:02:03:04:05:06"}},
	}

	called := false
	wake := func(mac net.HardwareAddr, addr string) error {
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
