package logincaptcha

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image/png"
	"regexp"
	"strings"
	"testing"

	corecaptcha "github.com/go-admin-team/go-admin-core/v2/captcha"
	"github.com/mojocn/base64Captcha"
)

func TestGenerate(t *testing.T) {
	for range 20 {
		id, data, err := Generate()
		if err != nil {
			t.Fatal(err)
		}
		answer := base64Captcha.DefaultMemStore.Get(id, false)
		if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(answer) {
			t.Fatalf("expected six digits, got %q", answer)
		}
		if !strings.HasPrefix(data, "data:image/png;base64,") {
			t.Fatal("expected PNG data URI")
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != 180 || img.Bounds().Dy() != 52 {
			t.Fatalf("unexpected image bounds: %v", img.Bounds())
		}
		if !corecaptcha.Verify(id, answer, true) {
			t.Fatal("generated challenge rejected by login verifier")
		}
		if corecaptcha.Verify(id, answer, true) {
			t.Fatal("challenge could be reused")
		}
	}
}

type failingStore struct{ base64Captcha.Store }

func (failingStore) Set(string, string) error { return errors.New("store unavailable") }

func TestGenerateStoreFailure(t *testing.T) {
	original := base64Captcha.DefaultMemStore
	base64Captcha.DefaultMemStore = failingStore{}
	t.Cleanup(func() { base64Captcha.DefaultMemStore = original })
	id, data, err := Generate()
	if err == nil || id != "" || data != "" {
		t.Fatal("must not issue an unstored challenge")
	}
}

func TestRenderLeadingZeros(t *testing.T) {
	data, err := render("000012")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/png;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	// Each of the six glyph columns must contain dark foreground pixels.
	for column := range 6 {
		dark := 0
		for x := 22 + column*26; x < 38+column*26; x++ {
			for y := 14; y < 39; y++ {
				r, g, b, _ := img.At(x, y).RGBA()
				if r < 100*257 && g < 100*257 && b < 130*257 {
					dark++
				}
			}
		}
		if dark == 0 {
			t.Fatalf("glyph %d is missing", column)
		}
	}
}
