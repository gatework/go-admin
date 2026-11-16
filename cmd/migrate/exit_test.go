package migrate

import (
	"context"
	"path/filepath"
	"testing"
)

// A deployment decides whether to start the new version on this command's
// exit code, so every failure is returned to the root command - which exits
// non-zero after the deferred closes here have run - rather than printed and
// followed by a successful exit, or ended with os.Exit from deep inside.
func TestRunReturnsAFailureForTheExitCode(t *testing.T) {
	previous := configYml
	t.Cleanup(func() { configYml = previous })
	configYml = filepath.Join(t.TempDir(), "missing.yml")
	if err := run(context.Background()); err == nil {
		t.Fatal("run with a missing configuration succeeded")
	}
	if err := runStatus(context.Background()); err == nil {
		t.Fatal("status with a missing configuration succeeded")
	}
}
