package cmd

import (
	"log"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trugamr/wol/scheduler"
	"github.com/trugamr/wol/server"
	"github.com/trugamr/wol/wake"
)

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

		// Start cron wake-ups when configured; a misconfigured schedule
		// (unknown machine or invalid cron) aborts serve before it listens.
		if len(cfg.Schedules) > 0 {
			sched, err := scheduler.New(cfg.Machines, cfg.Schedules, wake.Broadcast)
			cobra.CheckErr(err)
			sched.Start()
			log.Printf("Started %d scheduled wake-up(s)", len(cfg.Schedules))
		}

		srv := server.New(cfg, server.NewProbingPinger(cfg.Ping.Privileged), wake.Broadcast, server.BuildInfo{
			Version: version,
			Commit:  commit,
			Date:    date,
		})

		log.Printf("Listening on %s", cfg.Server.Listen)
		if err := http.ListenAndServe(cfg.Server.Listen, srv.Routes()); err != nil {
			cobra.CheckErr(err)
		}
	},
}
