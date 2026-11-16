package diagnostics

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/gatework/motor/app/net/models/diagnosticrun"
	"github.com/gatework/motor/common/account"
	"github.com/gatework/motor/common/errors"
)

// The per-user run limit, on top of the process-wide concurrency limit.
const (
	RunsPerWindow = 30
	RunWindow     = 10 * time.Minute
)

// RateLimitedErr is a user who has started RunsPerWindow diagnostics within
// the last RunWindow.
//
//revive:disable-next-line:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
var RateLimitedErr = errors.New(errors.KindUnavailable, "diagnostics.rate_limited", "") //nolint:errname // Project sentinel errors use Err suffix.

// Runs gates and records diagnostics in a tenant's database. The limit is
// shared by every replica, because it is counted in the database.
type Runs struct {
	DB  *gorm.DB
	Now func() time.Time
}

// Admit counts one run against owner's limit, or refuses it.
func (r Runs) Admit(ctx context.Context, owner Owner) error {
	limits := account.Service{DB: r.DB.WithContext(ctx), Now: r.Now}
	err := limits.TakeLimit(fmt.Sprintf("diagnostics.run:%d", owner.UserID), RunsPerWindow, RunWindow)
	if errors.Is(err, account.LimitedErr) {
		return RateLimitedErr
	}
	return err
}

// Record keeps the audit trail of a started job.
func (r Runs) Record(ctx context.Context, owner Owner, job Job) error {
	return r.DB.WithContext(ctx).Create(&diagnosticrun.Record{
		UserID: owner.UserID, JobID: job.ID, Tool: job.Tool, Target: job.Target, Family: job.Family,
		Mode: job.Mode, Port: job.Port, Command: job.Command, StartedAt: job.StartedAt,
	}).Error
}
