package handler

import (
	"strings"
	"testing"

	"github.com/gatework/motor/config"
)

func TestThirdUploadReportsAnUnconfiguredStore(t *testing.T) {
	previous := config.Ext()
	cleared := previous
	cleared.FileStore = config.FileStore{}
	config.SetExt(cleared)
	t.Cleanup(func() { config.SetExt(previous) })

	for source, want := range map[string]string{"2": "AliYunOSS", "3": "QiNiuKodo"} {
		err := thirdUpload(t.Context(), source, "x.png", "/tmp/x.png")
		if err == nil {
			t.Errorf("source=%s: no error from an unconfigured store", source)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("source=%s: error names %q, want it to mention %s", source, err, want)
		}
	}
}

// source 1 and anything unrecognised keep the local copy and do nothing else.
func TestThirdUploadIgnoresLocalAndUnknownSources(t *testing.T) {
	for _, source := range []string{"", "1", "9"} {
		if err := thirdUpload(t.Context(), source, "x.png", "/tmp/x.png"); err != nil {
			t.Errorf("source=%q returned %v, want nil", source, err)
		}
	}
}
