package jobs

import "github.com/gatework/motor-core/logger"

// RegisterBuiltinTasks registers the tasks this module ships. The host calls
// it once, before Setup.
func RegisterBuiltinTasks() {
	RegisterTask("ExamplesOne", ExamplesOne{})
}

// ExamplesOne demonstrates a scheduler target without an external side effect.
type ExamplesOne struct{}

// Exec logs execution without exposing the caller-supplied argument.
func (ExamplesOne) Exec(_ any) error {
	logger.NewHelper(logger.Default()).Info("example job completed")
	return nil
}
