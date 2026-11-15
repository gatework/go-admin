package jobs

import (
	"context"
	"testing"

	"github.com/gatework/motor-core/sdk"
	"github.com/gatework/motor-core/sdk/pkg/cronjob"
)

// Shutdown waits for the heartbeat and the scheduler with the caller's budget.
// When that budget is spent, releasing the lease with the same context fails
// at once and the successor has to wait out the whole TTL. The release gets its
// own short, independent deadline instead.
func TestShutdownReleasesTheLeaseEvenWhenItsBudgetIsSpent(t *testing.T) {
	const tenant = "spent-shutdown-budget"
	db := leaseDB(t)
	sdk.Runtime.SetCrontab(tenant, cronjob.NewWithSeconds())

	holder := newSupervisor(tenant, db, "instance-a")
	holder.tick()
	if !holder.holdsLease() {
		t.Fatal("the supervisor did not take a free lease")
	}
	close(holder.done) // this fixture ticks synchronously instead of running the heartbeat

	spent, cancel := context.WithCancel(t.Context())
	cancel()
	holder.shutdown(spent)

	if owner := readLease(t, db).Owner; owner != "" {
		t.Fatalf("lease still owned by %q after shutdown; the successor must wait out the TTL", owner)
	}
}
