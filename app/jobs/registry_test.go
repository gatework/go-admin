package jobs

import "testing"

func TestRegisterTaskRefusesDuplicatesAndLateRegistration(t *testing.T) {
	saved, started := tasks, schedulersStarted.Load()
	t.Cleanup(func() { tasks = saved; schedulersStarted.Store(started) })
	tasks = map[string]Task{}
	schedulersStarted.Store(false)

	RegisterTask("backup", ExamplesOne{})
	mustPanic(t, "duplicate name", func() { RegisterTask("backup", ExamplesOne{}) })
	mustPanic(t, "nil task", func() { RegisterTask("other", nil) })
	schedulersStarted.Store(true)
	mustPanic(t, "after Setup", func() { RegisterTask("late", ExamplesOne{}) })
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: did not panic", what)
		}
	}()
	f()
}
