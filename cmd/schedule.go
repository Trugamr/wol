package cmd

import (
	"fmt"
	"log"
	"net"

	"github.com/robfig/cron/v3"
	"github.com/trugamr/wol/config"
)

// resolvedSchedule is a config.Schedule that has been validated against the
// configured machines: its target MAC and broadcast address are resolved and its
// cron expression is parsed. label is what gets logged when the job runs.
type resolvedSchedule struct {
	label    string
	mac      net.HardwareAddr
	addr     string
	schedule cron.Schedule
}

// resolveSchedules validates each schedule in cfg and returns the resolved
// schedules ready to register with cron. It fails fast: the first schedule that
// references an unknown machine, has an unparseable MAC, or carries an invalid
// cron expression aborts with an error, so misconfiguration is surfaced at
// startup rather than silently never firing. The broadcast target is resolved
// per machine (the same precedence the send and web-wake paths use), so a
// scheduled wake honors any per-machine broadcast override.
func resolveSchedules(cfg *config.Config) ([]resolvedSchedule, error) {
	resolved := make([]resolvedSchedule, 0, len(cfg.Schedules))
	for _, s := range cfg.Schedules {
		label := s.Name
		if label == "" {
			label = s.Machine
		}

		machine, err := cfg.MachineByName(s.Machine)
		if err != nil {
			return nil, fmt.Errorf("schedule %q: %w", label, err)
		}

		mac, err := machine.HardwareAddr()
		if err != nil {
			return nil, fmt.Errorf("schedule %q: %w", label, err)
		}

		schedule, err := cron.ParseStandard(s.Cron)
		if err != nil {
			return nil, fmt.Errorf("schedule %q: invalid cron expression %q: %w", label, s.Cron, err)
		}

		resolved = append(resolved, resolvedSchedule{
			label:    label,
			mac:      mac,
			addr:     cfg.BroadcastFor(machine).Addr(),
			schedule: schedule,
		})
	}
	return resolved, nil
}

// wakeJob returns the function cron runs when a schedule fires: it logs the
// trigger and wakes the machine, logging any error. It is split out so the job
// body can be tested without waiting on cron timing.
func wakeJob(wake waker, rs resolvedSchedule) func() {
	return func() {
		log.Printf("Triggered scheduled wake for %s", rs.label)
		if err := wake(rs.mac, rs.addr); err != nil {
			log.Printf("Error sending scheduled magic packet for %s: %v", rs.label, err)
		}
	}
}

// scheduler runs cron wake-ups for the duration of the serve command.
type scheduler struct {
	cron *cron.Cron
}

// newScheduler resolves and registers every schedule in cfg, returning an error
// if any references an unknown machine, has an unparseable MAC, or has an invalid
// cron expression. The returned scheduler is not started yet; call start to begin
// firing jobs.
func newScheduler(cfg *config.Config, wake waker) (*scheduler, error) {
	resolved, err := resolveSchedules(cfg)
	if err != nil {
		return nil, err
	}

	c := cron.New()
	for _, rs := range resolved {
		// rs.schedule is already parsed, so register it directly rather than
		// re-parsing the spec string via AddFunc.
		c.Schedule(rs.schedule, cron.FuncJob(wakeJob(wake, rs)))
	}

	return &scheduler{cron: c}, nil
}

// start begins running the scheduled jobs in the background.
//
// There is intentionally no stop method: serve blocks on http.ListenAndServe
// for the life of the process, so the scheduler simply dies with it. Add a stop
// method (wrapping s.cron.Stop) when serve grows graceful shutdown.
func (s *scheduler) start() {
	s.cron.Start()
}
