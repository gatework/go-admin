package firewall

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gatework/motor/common/worker"

	"github.com/gatework/motor/app/net/models/lease"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	fm "github.com/gatework/firemon"
	adapter "github.com/gatework/firemon/adapter/netconnector"
	"github.com/gatework/firemon/config"
	native "github.com/gatework/firemon/device"
	"github.com/gatework/firemon/snapshot"
	"github.com/gatework/firemon/vendors"
	"github.com/gatework/go-net-connector/connector"
	models "github.com/gatework/motor/app/net/models/firewall"
	"github.com/gatework/motor/app/net/models/firewall/storage"
	"github.com/gatework/motor/app/net/service/deviceaccess"
)

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var collectionSourceErr = errors.New("firewall collection source changed")

//revive:enable:error-naming

// nativeDialer opens collection readers to devices with access.
func nativeDialer(access deviceaccess.Config) CollectionDialer {
	return func(ctx context.Context, source CollectionSource) (native.Reader, error) {
		factory := adapter.Factory{Resolve: func(ctx context.Context, id string) (adapter.Target, error) {
			if err := ctx.Err(); err != nil {
				return adapter.Target{}, err
			}
			if id != storage.DeviceKey(source.Instance.ID) || source.reason() != "" {
				return adapter.Target{}, collectionSourceErr
			}
			c, err := access.ConnectionConfig(source.Device.IP, source.Connection.Settings)
			if err != nil {
				return adapter.Target{}, err
			}
			c.MaxOutputBytes = config.DefaultMaxBytes
			c.MaxScriptOutputBytes = config.DefaultMaxBytes
			return adapter.Target{Metadata: fm.Metadata{Device: id, Vendor: source.Instance.Vendor, Context: source.Instance.NativeContext, Software: source.Device.OS}, Connection: c}, nil
		}}
		return factory.OpenReadOnly(ctx, storage.DeviceKey(source.Instance.ID))
	}
}

type collectionSession struct {
	worker    *CollectionWorker
	target    models.CollectionTarget
	source    CollectionSource
	attempted bool
	closed    bool
}

func (s *collectionSession) OpenReadOnly(ctx context.Context, id string) (native.Reader, error) {
	if id != storage.DeviceKey(s.target.InstanceID) {
		return nil, InvalidErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Recheck immediately before opening; publication performs a locked recheck.
	source, err := collectionSource(ctx, s.worker.s.Access, s.worker.s.DB, s.target.InstanceID, false)
	if err != nil {
		return nil, err
	}
	if source.fingerprint() != s.target.SourceHash || source.reason() != "" {
		return nil, collectionSourceErr
	}
	if err = s.worker.stage(ctx, s.target, "connecting"); err != nil {
		return nil, err
	}
	s.attempted = true
	reader, err := s.worker.dial(ctx, source)
	if reader == nil {
		s.closed = !hasCleanupFailure(err)
		return nil, err
	}
	return &collectionReader{Reader: reader, session: s}, err
}

// hasCleanupFailure reports whether a connector error is anywhere in err,
// joined or wrapped.
func hasCleanupFailure(err error) bool {
	_, ok := errors.AsType[*connector.Error](err)
	return ok
}

type collectionReader struct {
	native.Reader
	session  *collectionSession
	once     sync.Once
	closeErr error
}

func (r *collectionReader) Read(ctx context.Context, v native.View) (native.Capture, error) {
	if err := r.session.worker.stage(ctx, r.session.target, "reading"); err != nil {
		return native.Capture{}, err
	}
	return r.Reader.Read(ctx, v)
}
func (r *collectionReader) Close() error {
	r.once.Do(func() {
		r.closeErr = r.Reader.Close()
		r.session.closed = r.closeErr == nil
		// A durable closure acknowledgement allows recovery to release the lease
		// after a filesystem publication whose final SQL acknowledgement was lost.
		if r.session.closed {
			// io.Closer gives Close no context; the acknowledgement still gets a
			// bounded deadline of its own.
			ctx, cancel := worker.Cleanup(context.Background(), 5*time.Second)
			defer cancel()
			q := collectionOwned(r.session.worker.s.db(ctx), r.session.target).Update("session_closed", true)
			if q.Error != nil {
				r.closeErr = q.Error
			}
		}
	})
	return r.closeErr
}

type collectionParser struct {
	worker *CollectionWorker
	target models.CollectionTarget
}

func (p collectionParser) Parse(ctx context.Context, source config.Source, meta fm.Metadata) (*fm.Firewall, error) {
	if err := p.worker.stage(ctx, p.target, "parsing"); err != nil {
		return nil, err
	}
	return (vendors.Parser{}).Parse(ctx, source, meta)
}

type collectionStore struct {
	*storage.Store
	worker *CollectionWorker
	target models.CollectionTarget
}

func (s *collectionStore) Publish(ctx context.Context, fw *fm.Firewall, options snapshot.PublishOptions) (snapshot.PublishResult, error) {
	var result snapshot.PublishResult
	t := s.target
	if fw == nil || fw.Metadata().Device != storage.DeviceKey(t.InstanceID) || fw.Metadata().Vendor != t.Vendor || fw.Metadata().Context != t.NativeContext || options.PublicationID != publicationID(t) || options.Expected != t.ExpectedRevision || !options.Live {
		return result, InvalidErr
	}
	err := s.worker.s.db(ctx).Transaction(func(tx *gorm.DB) error {
		var b models.CollectionBatch
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&b, t.BatchID).Error; err != nil {
			return err
		}
		source, err := collectionSource(ctx, s.worker.s.Access, tx, t.InstanceID, true)
		if err != nil {
			return err
		}
		if source.fingerprint() != t.SourceHash || source.reason() != "" {
			return collectionSourceErr
		}
		var owned models.CollectionTarget
		if err := collectionOwned(tx, t).Clauses(clause.Locking{Strength: "UPDATE"}).First(&owned).Error; err != nil {
			return ConflictErr
		}
		if err := lease.Check(tx, lease.Lease{DeviceID: t.DeviceID, Token: t.ClaimToken}); err != nil {
			return ConflictErr
		}
		if err := tx.Model(&owned).Update("stage", "publishing").Error; err != nil {
			return err
		}
		result, err = s.Store.Publish(ctx, fw, options)
		return err
	})
	return result, err
}
