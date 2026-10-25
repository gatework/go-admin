package apis

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/logger"
	"github.com/go-admin-team/go-admin-core/v2/sdk/pkg"
	"go-admin/config"
)

func uploadRouter(t *testing.T) *gin.Engine {
	t.Helper()
	t.Chdir(t.TempDir())
	previous := config.ExtConfig.FileStore
	config.ExtConfig.FileStore = config.FileStore{}
	t.Cleanup(func() { config.ExtConfig.FileStore = previous })
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(gin.Recovery(), func(c *gin.Context) { c.Set(pkg.LoggerKey, logger.NewHelper(logger.DefaultLogger)) })
	r.POST("/upload", File{}.UploadFile)
	return r
}

func uploadResponse(t *testing.T, r http.Handler, req *http.Request, wantSuccess bool) json.RawMessage {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("expected exactly one JSON response, got HTTP%d %s: %v", w.Code, w.Body.String(), err)
	}
	if (result.Code == 200) != wantSuccess {
		t.Fatalf("unexpected upload response: %s", w.Body.String())
	}
	return result.Data
}

func formUpload(values url.Values) *http.Request {
	req := httptest.NewRequest("POST", "/upload", strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func pngDataURL() string {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestBase64UploadRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"", "not-a-data-url", "data:image/png;base64,%%%", "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("not an image"))} {
		t.Run(input, func(t *testing.T) {
			r := uploadRouter(t)
			uploadResponse(t, r, formUpload(url.Values{"type": {"3"}, "file": {input}}), false)
			files, _ := filepath.Glob("static/uploadfile/*")
			if len(files) > 0 {
				t.Fatal("invalid upload persisted files", files)
			}
		})
	}
}

func TestUploadErrorsAreSingleResponses(t *testing.T) {
	t.Run("local write fails", func(t *testing.T) {
		r := uploadRouter(t)
		if err := os.WriteFile("static", []byte("not a directory"), 0600); err != nil {
			t.Fatal(err)
		}
		uploadResponse(t, r, formUpload(url.Values{"type": {"3"}, "file": {pngDataURL()}}), false)
	})
	t.Run("cloud configuration missing", func(t *testing.T) {
		r := uploadRouter(t)
		uploadResponse(t, r, formUpload(url.Values{"type": {"3"}, "file": {pngDataURL()}, "source": {"2"}}), false)
		files, _ := filepath.Glob("static/uploadfile/*")
		if len(files) != 0 {
			t.Fatal("failed upload leaked files", files)
		}
	})
	t.Run("multi without multipart", func(t *testing.T) {
		r := uploadRouter(t)
		uploadResponse(t, r, formUpload(url.Values{"type": {"2"}}), false)
	})
	t.Run("empty multipart", func(t *testing.T) {
		r := uploadRouter(t)
		var body bytes.Buffer
		w := multipart.NewWriter(&body)
		_ = w.WriteField("type", "2")
		_ = w.Close()
		req := httptest.NewRequest("POST", "/upload", &body)
		req.Header.Set("Content-Type", w.FormDataContentType())
		uploadResponse(t, r, req, false)
	})
}

func TestBase64UploadReturnsActualImageType(t *testing.T) {
	r := uploadRouter(t)
	data := uploadResponse(t, r, formUpload(url.Values{"type": {"3"}, "file": {pngDataURL()}}), true)
	var file FileResponse
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file.Type != "image/png" || !strings.HasSuffix(file.Path, ".png") {
		t.Fatalf("incorrect image metadata: %+v", file)
	}
	if _, err := os.Stat(strings.TrimPrefix(file.Path, "/")); err != nil {
		t.Fatal(err)
	}
}

func TestUploadRequestHasSizeLimit(t *testing.T) {
	r := uploadRouter(t)
	req := formUpload(url.Values{"type": {"3"}, "file": {pngDataURL()}})
	req.ContentLength = 33 << 20
	uploadResponse(t, r, req, false)
}

func TestMultipartUploadsAndFailedBatchCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, tag, source string
		names             []string
		success           bool
	}{
		{"single", "1", "", []string{"notes.txt"}, true},
		{"multiple", "2", "", []string{"first.txt", "second.txt"}, true},
		{"single cloud missing", "1", "2", []string{"notes.txt"}, false},
		{"second write fails", "2", "", []string{"first.txt", "second." + strings.Repeat("x", 300)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := uploadRouter(t)
			var body bytes.Buffer
			form := multipart.NewWriter(&body)
			_ = form.WriteField("type", tc.tag)
			_ = form.WriteField("source", tc.source)
			for _, name := range tc.names {
				part, err := form.CreateFormFile("file", name)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = part.Write([]byte("test file"))
			}
			_ = form.Close()
			req := httptest.NewRequest("POST", "/upload", &body)
			req.Header.Set("Content-Type", form.FormDataContentType())
			data := uploadResponse(t, r, req, tc.success)
			files, _ := filepath.Glob("static/uploadfile/*")
			if !tc.success && len(files) != 0 {
				t.Fatal("failed batch retained files", files)
			}
			if tc.success {
				if len(files) != len(tc.names) {
					t.Fatal("successful upload missing files", files)
				}
				if tc.tag == "2" {
					var results []FileResponse
					if err := json.Unmarshal(data, &results); err != nil || len(results) != 2 {
						t.Fatalf("bad multiple response: %s", data)
					}
				}
			}
		})
	}
}

func TestBase64MediaTypeMustMatchContent(t *testing.T) {
	r := uploadRouter(t)
	uploadResponse(t, r, formUpload(url.Values{"type": {"3"}, "file": {strings.Replace(pngDataURL(), "image/png", "image/jpeg", 1)}}), false)
}

func TestUploadLimitAppliesWithoutContentLength(t *testing.T) {
	r := uploadRouter(t)
	req := httptest.NewRequest("POST", "/upload", strings.NewReader("type=3&file="+strings.Repeat("x", int(maxUploadRequestBytes))))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var result struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("streamed oversized request: HTTP%d %s (%v)", w.Code, w.Body.String(), err)
	}
	files, _ := filepath.Glob("static/uploadfile/*")
	if len(files) != 0 {
		t.Fatal("oversized upload persisted files", files)
	}
}

func TestUploadURLContract(t *testing.T) {
	for _, tag := range []string{"1", "2", "3"} {
		for _, source := range []string{"", "1"} {
			t.Run("type="+tag+"/source="+source, func(t *testing.T) {
				r := uploadRouter(t)
				var body bytes.Buffer
				form := multipart.NewWriter(&body)
				_ = form.WriteField("type", tag)
				_ = form.WriteField("source", source)
				if tag == "3" {
					_ = form.WriteField("file", pngDataURL())
				} else {
					part, err := form.CreateFormFile("file", "notes.txt")
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write([]byte("test file"))
				}
				_ = form.Close()
				req := httptest.NewRequest("POST", "http://localhost:8000/upload", &body)
				req.Header.Set("Content-Type", form.FormDataContentType())
				data := uploadResponse(t, r, req, true)
				var file FileResponse
				if tag == "2" {
					var files []FileResponse
					if err := json.Unmarshal(data, &files); err != nil || len(files) != 1 {
						t.Fatalf("bad files: %s", data)
					}
					file = files[0]
				} else if err := json.Unmarshal(data, &file); err != nil {
					t.Fatal(err)
				}
				if tag == "1" || source != "1" {
					if !strings.HasPrefix(file.Path, "/static/uploadfile/") || file.FullPath != file.Path {
						t.Fatalf("relative URL contract required for UI base concatenation: %+v", file)
					}
				} else if !strings.HasPrefix(file.Path, "static/uploadfile/") || file.FullPath != "http://localhost:8000/"+file.Path {
					t.Fatalf("source=1 multi/base64 absolute URL contract: %+v", file)
				}
			})
		}
	}
}

func TestBase64UploadSupportsRasterFormats(t *testing.T) {
	pixel := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var gifData, jpegData bytes.Buffer
	if err := gif.Encode(&gifData, pixel, nil); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegData, pixel, nil); err != nil {
		t.Fatal(err)
	}
	// A one-pixel VP8 WebP fixture, independent of the module cache/testdata.
	for _, tc := range []struct{ media, extension, payload string }{
		{"image/gif", ".gif", base64.StdEncoding.EncodeToString(gifData.Bytes())},
		{"image/jpeg", ".jpg", base64.StdEncoding.EncodeToString(jpegData.Bytes())},
		{"image/webp", ".webp", "UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA"},
	} {
		t.Run(tc.media, func(t *testing.T) {
			data, extension, err := decodeUploadImage("data:" + tc.media + ";base64," + tc.payload)
			if err != nil || extension != tc.extension || len(data) == 0 {
				t.Fatalf("valid format rejected: extension=%s error=%v", extension, err)
			}
		})
	}
}
