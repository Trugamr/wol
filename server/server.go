// Package server provides the wol web interface for listing and waking
// configured machines.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/trugamr/wol/config"
	"github.com/trugamr/wol/wake"
)

//go:embed templates/*
var templates embed.FS

// Pinger reports whether a network address is currently reachable.
type Pinger interface {
	Reachable(addr string) (bool, error)
}

// BuildInfo carries version metadata rendered in the web interface footer.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Server holds the dependencies for the web handlers. Injecting them (rather
// than reaching for package globals) lets tests substitute fakes for the pinger
// and wake action, so handlers can be exercised without real ICMP or UDP traffic.
type Server struct {
	cfg    *config.Config
	pinger Pinger
	waker  wake.Waker
	build  BuildInfo
}

// New creates a Server with its dependencies injected.
func New(cfg *config.Config, pinger Pinger, waker wake.Waker, build BuildInfo) *Server {
	return &Server{cfg: cfg, pinger: pinger, waker: waker, build: build}
}

// Routes registers all HTTP routes and returns the handler.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /wake", s.handleWake)
	mux.HandleFunc("GET /status", s.handleStatus)
	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Parse the template
	index, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Execute the template
	data := map[string]interface{}{
		"Machines":     s.cfg.Machines,
		"Version":      s.build.Version,
		"Commit":       s.build.Commit,
		"Date":         s.build.Date,
		"FlashMessage": consumeFlashMessage(w, r), // Get flash message from cookie
	}
	err = index.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// setFlashMessage sets a flash message in a cookie
func setFlashMessage(w http.ResponseWriter, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:  "flash",
		Value: message,
		Path:  "/",
	})
}

// consumeFlashMessage retrieves and clears the flash message from the request
func consumeFlashMessage(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("flash")
	if err == nil {
		// Clear the cookie
		http.SetCookie(w, &http.Cookie{
			Name:    "flash",
			Value:   "",
			Path:    "/",
			Expires: time.Now().Add(-1 * time.Hour),
		})

		return cookie.Value
	}
	return ""
}

func (s *Server) handleWake(w http.ResponseWriter, r *http.Request) {
	machineName := r.FormValue("name")
	mac, err := wake.ByName(s.cfg.Machines, machineName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("Sending magic packet to %s", mac)
	if err := s.waker(mac); err != nil {
		log.Printf("Error sending magic packet: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set flash message cookie
	setFlashMessage(w, fmt.Sprintf("Wake-up signal sent to %s. The machine should wake up shortly.", machineName))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// machineStatus returns the status of a single machine.
func (s *Server) machineStatus(machine config.Machine) (string, error) {
	if machine.IP == nil {
		return "unknown", nil
	}

	reachable, err := s.pinger.Reachable(*machine.IP)
	if err != nil {
		fmt.Println(err)
		return "unknown", err
	}
	if reachable {
		return "online", nil
	}

	return "offline", nil
}

// machinesStatus returns a map of machine names to their statuses concurrently.
func (s *Server) machinesStatus() map[string]string {
	var mu sync.Mutex
	statuses := make(map[string]string)
	var wg sync.WaitGroup

	for _, machine := range s.cfg.Machines {
		wg.Add(1)
		go func(machine config.Machine) {
			defer wg.Done()
			status, err := s.machineStatus(machine)
			if err != nil {
				log.Printf("Error getting status for machine %s: %v", machine.Name, err)
				return
			}

			mu.Lock()
			statuses[machine.Name] = status
			mu.Unlock()
		}(machine)
	}

	wg.Wait()

	return statuses
}

// writeMachinesStatus writes a single SSE frame with the current status of all
// machines to w, flushing if w supports it. Extracted from the streaming loop so
// the frame can be tested without running the loop.
func (s *Server) writeMachinesStatus(w io.Writer) error {
	statuses := s.machinesStatus()
	data, err := json.Marshal(statuses)
	if err != nil {
		return fmt.Errorf("error marshaling status: %w", err)
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("error writing status: %w", err)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	return nil
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	send := func() {
		if err := s.writeMachinesStatus(w); err != nil {
			log.Print(err)
		}
	}

	// Sends initial status
	send()

	// Send status updates every few seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

// probingPinger is the production Pinger, backed by pro-bing.
type probingPinger struct {
	privileged bool
}

// NewProbingPinger returns the production Pinger, backed by pro-bing.
func NewProbingPinger(privileged bool) *probingPinger {
	return &probingPinger{privileged: privileged}
}

func (p *probingPinger) Reachable(addr string) (bool, error) {
	pinger, err := probing.NewPinger(addr)
	if err != nil {
		return false, fmt.Errorf("error creating pinger: %w", err)
	}
	// Set privileged mode based on config
	pinger.SetPrivileged(p.privileged)

	// We only want to ping once and wait 2 seconds for a response
	pinger.Timeout = 2 * time.Second
	pinger.Count = 1

	err = pinger.Run()
	if err != nil {
		return false, fmt.Errorf("error pinging: %w", err)
	}

	// If we receive even a single packet, the address is reachable
	return pinger.Statistics().PacketsRecv > 0, nil
}
