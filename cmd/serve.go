package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
	"github.com/spf13/cobra"
	"github.com/trugamr/wol/config"
	"github.com/trugamr/wol/magicpacket"
)

//go:embed templates/*
var templates embed.FS

func init() {
	rootCmd.AddCommand(serveCmd)
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Serve a web interface to wake up machines",
	Long:  "Serve a web interface that lists all the configured machines and allows you to wake them up",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if sources := cfg.Sources(); len(sources) > 0 {
			log.Printf("Loaded config from %s", strings.Join(sources, ", "))
		} else {
			log.Print("No config file found; using built-in defaults")
		}

		handler := newServer(cfg, newProbingPinger(cfg.Ping.Privileged), broadcastWake).routes()

		log.Printf("Listening on %s", cfg.Server.Listen)
		err := http.ListenAndServe(cfg.Server.Listen, handler)
		if err != nil {
			cobra.CheckErr(err)
		}
	},
}

// Pinger reports whether a network address is currently reachable.
type Pinger interface {
	Reachable(addr string) (bool, error)
}

// waker sends a wake-on-LAN magic packet to the given MAC address, broadcasting
// to addr (a "host:port" target).
type waker func(mac net.HardwareAddr, addr string) error

// broadcastWake is the production waker: it broadcasts a real magic packet.
func broadcastWake(mac net.HardwareAddr, addr string) error {
	return magicpacket.NewMagicPacket(mac).Broadcast(addr)
}

// server holds the dependencies for the web handlers. Injecting them (rather
// than reaching for package globals) lets tests substitute fakes for the pinger
// and wake action, so handlers can be exercised without real ICMP or UDP traffic.
type server struct {
	cfg    *config.Config
	pinger Pinger
	wake   waker
}

func newServer(cfg *config.Config, pinger Pinger, wake waker) *server {
	return &server{cfg: cfg, pinger: pinger, wake: wake}
}

// routes registers all HTTP routes and returns the handler.
func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("POST /wake", s.handleWake)
	mux.HandleFunc("GET /status", s.handleStatus)
	return mux
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
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
		"Version":      version,
		"Commit":       commit,
		"Date":         date,
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

func (s *server) handleWake(w http.ResponseWriter, r *http.Request) {
	machineName := r.FormValue("name")
	machine, err := s.cfg.MachineByName(machineName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mac, err := net.ParseMAC(machine.Mac)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to parse MAC address: %v", err), http.StatusInternalServerError)
		return
	}

	addr := s.cfg.BroadcastFor(machine).Addr()
	log.Printf("Sending magic packet to %s via %s", mac, addr)
	if err := s.wake(mac, addr); err != nil {
		log.Printf("Error sending magic packet: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Set flash message cookie
	setFlashMessage(w, fmt.Sprintf("Wake-up signal sent to %s. The machine should wake up shortly.", machineName))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// machineStatus returns the status of a single machine.
func (s *server) machineStatus(machine config.Machine) (string, error) {
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
func (s *server) machinesStatus() map[string]string {
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
func (s *server) writeMachinesStatus(w io.Writer) error {
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

func (s *server) handleStatus(w http.ResponseWriter, r *http.Request) {
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

func newProbingPinger(privileged bool) *probingPinger {
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
