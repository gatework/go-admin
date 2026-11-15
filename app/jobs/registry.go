package jobs

import (
	"fmt"
	"sync/atomic"
)

// tasks maps a sys_job invoke target to the Task it runs. It is written only
// during start-up, before Setup starts any scheduler, and read-only afterwards,
// so the scheduler goroutines read it without a lock.
var tasks = map[string]Task{}

// schedulersStarted closes registration: a task registered after schedulers
// run would be read concurrently with the write.
var schedulersStarted atomic.Bool

// RegisterTask makes task runnable as the invoke target name of a task job
// (sys_job.job_type = TypeTask). Call it during start-up, before Setup. An
// empty or repeated name, a nil task, or a registration after Setup is a
// programming error and panics.
func RegisterTask(name string, task Task) {
	switch {
	case schedulersStarted.Load():
		panic(fmt.Sprintf("jobs: task %q registered after the schedulers started", name))
	case name == "" || task == nil:
		panic("jobs: a task needs a name and an implementation")
	case tasks[name] != nil:
		panic(fmt.Sprintf("jobs: task %q registered twice", name))
	}
	tasks[name] = task
}
