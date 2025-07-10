package cmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
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
		mux := http.NewServeMux()

		mux.HandleFunc("GET /{$}", handleIndex)
		mux.HandleFunc("POST /wake", handleWake)
		mux.HandleFunc("GET /status", handleStatus)

		log.Printf("Listening on %s", cfg.Server.Listen)
		err := http.ListenAndServe(cfg.Server.Listen, mux)
		if err != nil {
			cobra.CheckErr(err)
		}
	},
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Parse the template
	index, err := template.ParseFS(templates, "templates/index.html")
	if err != nil {
		log.Printf("Error parsing template: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Execute the template
	data := map[string]interface{}{
		"Machines":     cfg.Machines,
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

func handleWake(w http.ResponseWriter, r *http.Request) {
	machineName := r.FormValue("name")
	machine, err := getMachineByName(machineName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch machine.WakeMethod() {
	case config.WakeMethodHTTP:
		if machine.HTTP == nil {
			message := fmt.Sprintf("WakeMethod was determined to be HTTP, but no HTTP configuration was found for machine %s", machine.Name)
			log.Print(message)
			http.Error(w, message, http.StatusInternalServerError)
			return
		}

		log.Printf("Sending %s request to %s", machine.HTTP.Method, machine.HTTP.Endpoint)
		var body *strings.Reader
		if machine.HTTP.Body != "" {
			body = strings.NewReader(machine.HTTP.Body)
			log.Printf("Request body: %s", machine.HTTP.Body)
		}
		req, err := http.NewRequest(machine.HTTP.Method, machine.HTTP.Endpoint, body)
		if err != nil {
			log.Printf("Error constructing HTTP request: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Error sending HTTP request: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		message := fmt.Sprintf("HTTP request returned status code %d", resp.StatusCode)
		log.Print(message)
		if resp.StatusCode != http.StatusOK {
			http.Error(w, message, http.StatusInternalServerError)
			return
		}
	default:
		mac, err := getMac(machine)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("Sending magic packet to %s", mac)
		mp := magicpacket.NewMagicPacket(mac)
		if err := mp.Broadcast(); err != nil {
			log.Printf("Error sending magic packet: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}		
	}

	// Set flash message cookie
	setFlashMessage(w, fmt.Sprintf("Wake-up signal sent to %s. The machine should wake up shortly.", machineName))

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// getMachineStatus returns the status of a machine
func getMachineStatus(machine config.Machine) (string, error) {
	if machine.IP == nil {
		return "unknown", nil
	}

	reachable, err := isAddressReachable(*machine.IP)
	if err != nil {
		return "unknown", err
	}
	if reachable {
		return "online", nil
	}

	return "offline", nil
}

// getMachinesStatus returns a map of machine names to their statuses concurrently
func getMachinesStatus() map[string]string {
	var mu sync.Mutex
	statuses := make(map[string]string)
	var wg sync.WaitGroup

	for _, machine := range cfg.Machines {
		wg.Add(1)
		go func(machine config.Machine) {
			defer wg.Done()
			status, err := getMachineStatus(machine)
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

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Sends the current status of all machines
	sendMachinesStatus := func() {
		statuses := getMachinesStatus()
		data, err := json.Marshal(statuses)
		if err != nil {
			log.Printf("Error marshaling status: %v", err)
			return
		}

		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil {
			log.Printf("Error writing status: %v", err)
			return
		}

		w.(http.Flusher).Flush()
	}

	// Sends initial status
	sendMachinesStatus()

	// Send status updates every few seconds
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			sendMachinesStatus()
		}
	}
}

func isAddressReachable(addr string) (bool, error) {
	pinger, err := probing.NewPinger(addr)
	if err != nil {
		return false, fmt.Errorf("error creating pinger: %v", err)
	}
	// Set privileged mode based on config
	pinger.SetPrivileged(cfg.Ping.Privileged)

	// We only want to ping once and wait 2 seconds for a response
	pinger.Timeout = 2 * time.Second
	pinger.Count = 1

	err = pinger.Run()
	if err != nil {
		return false, fmt.Errorf("error pinging: %v", err)
	}

	// If we receive even a single packet, the address is reachable
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return false, nil
	}

	return true, nil
}
