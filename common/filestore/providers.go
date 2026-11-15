package filestore

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
	"github.com/huaweicloud/huaweicloud-sdk-go-obs/obs"
	"github.com/qiniu/go-sdk/v7/auth/qbox"
	"github.com/qiniu/go-sdk/v7/client"
	"github.com/qiniu/go-sdk/v7/storage"
)

func uploadTransport() *http.Transport {
	return &http.Transport{Proxy: http.ProxyFromEnvironment, TLSHandshakeTimeout: 10 * time.Second}
}

func qiniuZone(zone Zone) (*storage.Zone, error) {
	switch zone {
	case "", EastChina:
		return &storage.ZoneHuadong, nil
	case NorthChina:
		return &storage.ZoneHuabei, nil
	case SouthChina:
		return &storage.ZoneHuanan, nil
	case NorthAmerica:
		return &storage.ZoneBeimei, nil
	case Singapore:
		return &storage.ZoneXinjiapo, nil
	default:
		return nil, fmt.Errorf("filestore: unknown qiniu zone %q", zone)
	}
}

func (s *Store) uploadAliyun(ctx context.Context, name, path string) error {
	transport := uploadTransport()
	defer transport.CloseIdleConnections()
	client, err := oss.New(
		s.config.Endpoint,
		s.config.AccessKeyID,
		s.config.AccessKeySecret,
		oss.HTTPClient(&http.Client{Transport: transport}),
	)
	if err != nil {
		return fmt.Errorf("filestore: aliyun client: %w", err)
	}
	bucket, err := client.Bucket(s.config.BucketName)
	if err != nil {
		return fmt.Errorf("filestore: aliyun bucket: %w", err)
	}
	if err := bucket.UploadFile(name, path, 100*1024, oss.Routines(3), oss.Checkpoint(true, ""), oss.WithContext(ctx)); err != nil {
		return fmt.Errorf("filestore: aliyun upload: %w", err)
	}
	return nil
}

func (s *Store) uploadHuawei(ctx context.Context, name, path string) error {
	transport := uploadTransport()
	defer transport.CloseIdleConnections()
	client, err := obs.New(
		s.config.AccessKeyID,
		s.config.AccessKeySecret,
		s.config.Endpoint,
		obs.WithRequestContext(ctx),
		// SDK retry backoff uses time.Sleep and ignores request cancellation.
		obs.WithMaxRetryCount(0),
		obs.WithHttpTransport(transport),
	)
	if err != nil {
		return fmt.Errorf("filestore: huawei client: %w", err)
	}
	defer client.Close()
	input := &obs.PutFileInput{}
	input.Bucket = s.config.BucketName
	input.Key = name
	input.SourceFile = path
	if _, err := client.PutFile(input); err != nil {
		return fmt.Errorf("filestore: huawei upload: %w", err)
	}
	return nil
}

func (s *Store) uploadQiniu(ctx context.Context, name, path string) error {
	transport := uploadTransport()
	defer transport.CloseIdleConnections()
	uploader := storage.NewFormUploaderEx(s.qiniu, &client.Client{Client: &http.Client{Transport: transport}})
	ttl := uint64(s.config.TokenTTL / time.Second)
	if s.config.TokenTTL%time.Second != 0 {
		ttl++
	}
	policy := storage.PutPolicy{
		Scope:   s.config.BucketName,
		Expires: ttl,
	}
	token := policy.UploadToken(qbox.NewMac(s.config.AccessKeyID, s.config.AccessKeySecret))
	var result storage.PutRet
	if err := uploader.PutFile(ctx, &result, token, name, path, nil); err != nil {
		return fmt.Errorf("filestore: qiniu upload: %w", err)
	}
	return nil
}
