package jobs

import "github.com/gatework/motor-core/logger"

// InitJob registers the built-in job implementations before schedulers start.
func InitJob() {
	jobList = map[string]JobExec{"ExamplesOne": ExamplesOne{}}
}

// ExamplesOne demonstrates a scheduler target without an external side effect.
type ExamplesOne struct{}

// Exec logs execution without exposing the caller-supplied argument.
func (ExamplesOne) Exec(_ any) error {
	logger.NewHelper(logger.Default()).Info("example job completed")
	return nil
}
