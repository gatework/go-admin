package filestore

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qiniu/go-sdk/v7/storage"
)

func testConfig(driver Driver, endpoint string) Config {
	return Config{
		Driver: driver, Endpoint: endpoint, AccessKeyID: "test-id",
		AccessKeySecret: "test-secret", BucketName: "test-bucket",
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, driver := range []Driver{Aliyun, Huawei, Qiniu} {
		t.Run(string(driver), func(t *testing.T) {
			_, err := New(Config{Driver: driver})
			var missing *NotConfiguredError
			if !errors.As(err, &missing) || missing.Driver != driver {
				t.Fatalf("missing configuration: %v", err)
			}
		})
	}
	if _, err := New(Config{Driver: "unknown"}); err == nil {
		t.Fatal("unknown provider accepted")
	}
	config := testConfig(Qiniu, "https://upload.example")
	config.Zone = "unknown"
	if _, err := New(config); err == nil {
		t.Fatal("unknown qiniu region accepted")
	}
	config.Zone = NorthChina
	config.TokenTTL = -time.Second
	if _, err := New(config); err == nil {
		t.Fatal("negative token ttl accepted")
	}
}

func TestQiniuUsesConfiguredRegion(t *testing.T) {
	for _, tc := range []struct {
		name string
		zone Zone
		want *storage.Zone
	}{
		{name: "default", want: &storage.ZoneHuadong},
		{name: "east china", zone: EastChina, want: &storage.ZoneHuadong},
		{name: "north china", zone: NorthChina, want: &storage.ZoneHuabei},
		{name: "south china", zone: SouthChina, want: &storage.ZoneHuanan},
		{name: "north america", zone: NorthAmerica, want: &storage.ZoneBeimei},
		{name: "singapore", zone: Singapore, want: &storage.ZoneXinjiapo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := testConfig(Qiniu, "https://upload.example")
			config.Zone = tc.zone
			store, err := New(config)
			if err != nil {
				t.Fatal(err)
			}
			if store.qiniu.Zone != tc.want || !store.qiniu.UseHTTPS {
				t.Fatalf("wrong upload configuration: %+v", store.qiniu)
			}
		})
	}
}

func TestCanceledUploadDoesNotOpenFileOrConnect(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, driver := range []Driver{Aliyun, Huawei, Qiniu} {
		store, err := New(testConfig(driver, "http://127.0.0.1:1"))
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Upload(ctx, "img/test.png", "/missing-file"); !errors.Is(err, context.Canceled) {
			t.Fatalf("provider %s did not preserve cancellation: %v", driver, err)
		}
	}
}

func TestInFlightUploadObservesCancellation(t *testing.T) {
	for _, driver := range []Driver{Aliyun, Huawei} {
		t.Run(string(driver), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}))
			defer server.Close()
			defer close(release)
			store, err := New(testConfig(driver, server.URL))
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "upload.txt")
			if err := os.WriteFile(file, []byte("test upload"), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- store.Upload(ctx, "upload.txt", file) }()
			select {
			case <-started:
			case err := <-done:
				t.Fatalf("upload never reached provider: %v", err)
			case <-time.After(3 * time.Second):
				t.Fatal("upload never reached provider")
			}
			cancel()
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("cancelled upload reported success")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("upload ignored caller cancellation")
			}
		})
	}
}
