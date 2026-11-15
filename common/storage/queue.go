package storage

import (
	"context"

	"github.com/gatework/motor-core/sdk/config"
	corestorage "github.com/gatework/motor-core/storage"
)

func openQueue(ctx context.Context, options *config.Queue) (corestorage.Queue, error) {
	if options == nil {
		options = &config.Queue{}
	}
	ctx, cancel := context.WithTimeout(ctx, storageIOTimeout)
	defer cancel()
	return options.Open(ctx)
}
