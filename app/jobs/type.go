package jobs

import "github.com/robfig/cron/v3"

// Job carries job state for this module.
type Job interface {
	Run()
	addJob(scheduler *cron.Cron) (int, error)
}

// Task carries job exec state for this module.
type Task interface {
	Exec(arg any) error
}
