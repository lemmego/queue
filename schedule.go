package queue

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/lemmego/api/app"
	"github.com/lemmego/tasker/scheduler"
)

// Scheduler returns the recurring-job scheduler, or nil if none was configured.
//
// It is registered in the container whenever Provider.Schedule is set, so a
// handler or a command can inspect what is scheduled — List() is the honest
// answer to "what runs on a timer here", which is otherwise a grep.
func Scheduler(a app.App) *scheduler.Scheduler {
	service := a.Service((*scheduler.Scheduler)(nil))
	sched, _ := service.(*scheduler.Scheduler)
	return sched
}

// buildScheduler registers the application's recurring jobs.
//
// It does not start anything. A scheduler that started wherever it was
// constructed would run in every web process and every worker, so an hourly job
// would fire once per instance — and nothing about that failure is visible from
// inside one process. Starting is the dedicated command's job, and running
// exactly one of those is a deployment decision the framework cannot make.
func (p *Provider) buildScheduler(a app.App) (*scheduler.Scheduler, error) {
	if p.Schedule == nil {
		return nil, nil
	}

	p.mu.RLock()
	mgr := p.manager
	p.mu.RUnlock()
	if mgr == nil {
		return nil, nil
	}

	sched := scheduler.New(mgr)
	for _, job := range p.Schedule(a) {
		if err := sched.Register(job); err != nil {
			// A malformed cron expression is a startup failure, not a job that
			// silently never runs. The alternative is discovering on Tuesday
			// that Monday's rollup has never fired.
			return nil, err
		}
	}

	p.mu.Lock()
	p.scheduler = sched
	p.mu.Unlock()
	return sched, nil
}

// ScheduleCommand runs the scheduler in the foreground.
//
// One process, deliberately. tasker's scheduler has no leader election, so two
// of these dispatch every job twice. Run it as a single replica; for jobs where
// a double dispatch would be harmful anyway — a weekly rollup, an email — take a
// cache lock inside the job as well, because "exactly one process" is a promise
// a deployment makes and occasionally breaks.
func ScheduleCommand(a app.App) *cobra.Command {
	return &cobra.Command{
		Use:   "tasker:schedule",
		Short: "Run the recurring-job scheduler",
		Long: "Dispatches the jobs registered in queue.Provider.Schedule onto the queue " +
			"on their cron expressions. Workers execute them; this process only " +
			"dispatches, so run exactly one of it.",
		Run: func(cmd *cobra.Command, args []string) {
			sched := Scheduler(a)
			if sched == nil {
				slog.Error("queue: nothing is scheduled; set queue.Provider.Schedule")
				os.Exit(1)
			}

			jobs := sched.List()
			if len(jobs) == 0 {
				slog.Error("queue: the scheduler has no jobs registered")
				os.Exit(1)
			}

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := sched.Start(ctx); err != nil {
				slog.Error("queue: the scheduler failed to start", "error", err)
				os.Exit(1)
			}
			for _, job := range jobs {
				slog.Info("queue: scheduled", "id", job.ID, "schedule", job.Schedule, "queue", job.Queue)
			}

			signals := make(chan os.Signal, 1)
			signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
			<-signals

			slog.Info("queue: stopping the scheduler")
			if err := sched.Stop(context.Background()); err != nil {
				slog.Error("queue: the scheduler did not stop cleanly", "error", err)
			}
		},
	}
}
