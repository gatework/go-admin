// Package filestore uploads locally persisted files to object storage.
package filestore

import (
	"context"
	"fmt"
	"time"

	"github.com/qiniu/go-sdk/v7/storage"
)

// Driver identifies the configured object store.
type Driver string

// Supported values for this contract.
const (
	Aliyun Driver = "AliYunOSS"
	Huawei Driver = "HuaweiOBS"
	Qiniu  Driver = "QiNiuKodo"
)

// Zone identifies a Qiniu storage region.
type Zone string

// Supported values for this contract.
const (
	EastChina    Zone = "HuaDong"
	NorthChina   Zone = "HuaBei"
	SouthChina   Zone = "HuaNan"
	NorthAmerica Zone = "BeiMei"
	Singapore    Zone = "XinJiaPo"
)

// Config contains typed provider options. Zone and TokenTTL apply to Qiniu.
type Config struct {
	Driver          Driver
	Endpoint        string
	AccessKeyID     string
	AccessKeySecret string
	BucketName      string
	Zone            Zone
	TokenTTL        time.Duration
}

// NotConfiguredError reports missing credentials or bucket configuration.
type NotConfiguredError struct {
	Driver Driver
}

// Error handles the error operation.
func (e *NotConfiguredError) Error() string {
	return fmt.Sprintf("file store %s is not configured; set it under extend.fileStore", e.Driver)
}

// Store owns immutable provider configuration. Each upload owns and closes its
// HTTP transport, so one request never retires another request's connections.
type Store struct {
	config Config
	qiniu  *storage.Config
}

// New validates configuration without connecting to the provider.
func New(config Config) (*Store, error) {
	switch config.Driver {
	case Aliyun, Huawei, Qiniu:
	default:
		return nil, fmt.Errorf("filestore: unsupported driver %q", config.Driver)
	}
	if config.Endpoint == "" || config.AccessKeyID == "" || config.AccessKeySecret == "" || config.BucketName == "" {
		return nil, &NotConfiguredError{Driver: config.Driver}
	}
	store := &Store{config: config}
	if config.Driver == Qiniu {
		zone, err := qiniuZone(config.Zone)
		if err != nil {
			return nil, err
		}
		if config.TokenTTL < 0 {
			return nil, fmt.Errorf("filestore: token ttl must not be negative")
		}
		if config.TokenTTL == 0 {
			config.TokenTTL = time.Hour
		}
		store.config = config
		store.qiniu = &storage.Config{Zone: zone, UseHTTPS: true}
	}
	return store, nil
}

// Upload sends path to name, preserving caller cancellation. The operation has
// a two-minute upper bound even when the caller supplied no deadline.
func (s *Store) Upload(ctx context.Context, name, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.config.Driver == "" {
		return &NotConfiguredError{}
	}
	if name == "" || path == "" {
		return fmt.Errorf("filestore: object name and file path are required")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	switch s.config.Driver {
	case Aliyun:
		return s.uploadAliyun(ctx, name, path)
	case Huawei:
		return s.uploadHuawei(ctx, name, path)
	case Qiniu:
		return s.uploadQiniu(ctx, name, path)
	default:
		return fmt.Errorf("filestore: unsupported driver %q", s.config.Driver)
	}
}
