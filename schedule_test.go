package queue

import (
	"context"
	"testing"

	"github.com/lemmego/api/app"
	"github.com/lemmego/tasker"
	"github.com/lemmego/tasker/scheduler"
)

// noopJob is a job that does nothing, so these tests are about the wiring.
type noopJob struct{}

func (j *noopJob) Handle(ctx context.Context) error { return nil }

// A provider with no Schedule registers no scheduler, so an application that
// does not want one pays nothing and Scheduler() answers honestly.
func TestNothingIsScheduledByDefault(t *testing.T) {
	p := &Provider{}
	sched, err := p.buildScheduler(nil)
	if err != nil {
		t.Fatalf("building with no schedule: %v", err)
	}
	if sched != nil {
		t.Error("a scheduler was built for a provider that declared none")
	}
}

// A malformed cron expression fails at startup. The alternative is finding out
// on Tuesday that Monday's job has never once run.
func TestAMalformedCronExpressionFailsStartup(t *testing.T) {
	p := &Provider{
		manager: tasker.NewConfiguredManager(tasker.Config{}),
		Schedule: func(a app.App) []scheduler.ScheduledJob {
			return []scheduler.ScheduledJob{
				{ID: "broken", Schedule: "not a cron expression", Job: &noopJob{}},
			}
		},
	}

	if _, err := p.buildScheduler(nil); err == nil {
		t.Error("a malformed schedule was accepted")
	}
}

// Registering does not start: a scheduler that started where it was built would
// dispatch once per process.
func TestRegisteringDoesNotStartTheScheduler(t *testing.T) {
	p := &Provider{
		manager: tasker.NewConfiguredManager(tasker.Config{}),
		Schedule: func(a app.App) []scheduler.ScheduledJob {
			return []scheduler.ScheduledJob{
				{ID: "weekly", Schedule: "5 0 * * 1", Job: &noopJob{}},
			}
		},
	}

	sched, err := p.buildScheduler(nil)
	if err != nil {
		t.Fatal(err)
	}
	if sched == nil {
		t.Fatal("no scheduler was built")
	}
	jobs := sched.List()
	if len(jobs) != 1 || jobs[0].ID != "weekly" {
		t.Errorf("registered %v", jobs)
	}

	// Stop on something never started is a no-op rather than an error, which is
	// what makes a command's deferred shutdown safe.
	if err := sched.Stop(context.Background()); err != nil {
		t.Errorf("Stop on an unstarted scheduler: %v", err)
	}
}
