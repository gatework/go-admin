package jobs

import "github.com/robfig/cron/v3"

// Job carries job state for this module.
type Job interface {
	Run()
	addJob(scheduler *cron.Cron) (int, error)
}

// JobExec carries job exec state for this module.
type JobExec interface {
	Exec(arg any) error
}

// CallExec provides call exec for this module.
func CallExec(e JobExec, arg any) error {
	return e.Exec(arg)
}
