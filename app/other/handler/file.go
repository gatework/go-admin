package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // Register GIF for uploaded images.
	_ "image/jpeg" // Register JPEG for uploaded images.
	_ "image/png"  // Register PNG for uploaded images.
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	_ "golang.org/x/image/webp" // Register WebP for bounded image decoding.

	"github.com/gatework/motor-core/response"
	"github.com/gatework/motor-core/sdk/api"
	"github.com/gatework/motor/common/filestore"
	"github.com/gatework/motor/config"
)

// FileResponse carries file response state for this module.
type FileResponse struct {
	Size     int64  `json:"size"`
	Path     string `json:"path"`
	FullPath string `json:"full_path"`
	Name     string `json:"name"`
	Type     string `json:"type"`
}

const path = "static/uploadfile/"

// File carries file state for this module.
type File struct{}

const maxUploadRequestBytes int64 = 32 << 20

//revive:disable:error-naming Project sentinel errors use Err suffix; see docs/standards.md.
//nolint:errname,staticcheck // ST1012: project sentinel errors use Err suffix; see docs/standards.md.
var invalidUploadErr = errors.New("invalid upload")

//revive:enable:error-naming

// UploadFile 上传图片
// @Summary 上传图片
// @Description 获取JSON
// @Tags 公共接口
// @Accept multipart/form-data
// @Param type query string true "type" (1：单图，2：多图, 3：base64图片)
// @Param file formData file true "file"
// @Success 200 {string} string	"{"code": 200, "message": "添加成功"}"
// @Success 200 {string} string	"{"code": -1, "message": "添加失败"}"
// @Router /api/v1/public/uploadFile [post]
// @Security Bearer
// Upload requests share one response boundary: helpers never write HTTP output.
// Local files created by a failed request are removed before returning.
func (File) UploadFile(c *gin.Context) {
	e, err := api.New(c, api.Dependencies{})
	if err != nil {
		response.Error(c, 500, err, "上传初始化失败")
		return
	}
	if c.Request.ContentLength > maxUploadRequestBytes {
		e.Error(http.StatusRequestEntityTooLarge, invalidUploadErr, "上传内容超过32 MiB限制")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadRequestBytes)
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		err = c.Request.ParseMultipartForm(8 << 20)
		if c.Request.MultipartForm != nil {
			defer func() {
				if err := c.Request.MultipartForm.RemoveAll(); err != nil {
					e.Logger.Warnf("cleanup multipart files: %v", err)
				}
			}()
		}
	} else {
		err = c.Request.ParseForm()
	}
	if err != nil {
		code := http.StatusUnprocessableEntity
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			code = http.StatusRequestEntityTooLarge
		}
		e.Error(code, err, "上传参数错误")
		return
	}
	source := c.PostForm("source")
	tag := c.PostForm("type")
	if err = validateUploadSource(source); err != nil {
		e.Error(500, err, "上传存储未配置")
		return
	}
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	urlPrefix := scheme + "://" + c.Request.Host
	batch := uploadBatch{source: source, tag: tag, urlPrefix: urlPrefix}
	succeeded := false
	defer func() {
		if !succeeded {
			batch.discard(func(err error) { e.Logger.Warnf("cleanup upload: %v", err) })
		}
	}()

	result, err := batch.save(c)
	if err == nil {
		for _, filename := range batch.saved {
			if err = thirdUpload(c.Request.Context(), source, filepath.Base(filename), filename); err != nil {
				break
			}
		}
	}
	if err != nil {
		code := 500
		if errors.Is(err, invalidUploadErr) {
			code = http.StatusUnprocessableEntity
		}
		e.Error(code, err, "上传失败")
		return
	}
	succeeded = true
	e.OK(result, "上传成功")
}

func validateUploadSource(source string) error {
	switch source {
	case "2":
		if !config.ExtConfig.FileStore.AliYun.Configured() {
			return errors.New("AliYunOSS file store is not configured")
		}
	case "3":
		if !config.ExtConfig.FileStore.QiNiu.Configured() {
			return errors.New("QiNiuKodo file store is not configured")
		}
	}
	return nil
}

func decodeUploadImage(input string) ([]byte, string, error) {
	metadata, payload, ok := strings.Cut(input, ",")
	if !ok || !strings.HasPrefix(metadata, "data:") || !strings.HasSuffix(metadata, ";base64") {
		return nil, "", fmt.Errorf("%w: expected an image data URL", invalidUploadErr)
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSuffix(strings.TrimPrefix(metadata, "data:"), ";base64"))
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid media type", invalidUploadErr)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid base64", invalidUploadErr)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid image", invalidUploadErr)
	}
	formats := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}
	expected, ok := formats[format]
	if !ok || mediaType != expected {
		return nil, "", fmt.Errorf("%w: image media type does not match content", invalidUploadErr)
	}
	if format == "jpeg" {
		format = "jpg"
	}
	return data, "." + format, nil
}

func uploadContentType(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	var head [512]byte
	n, err := file.Read(head[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(head[:n]), nil
}

// thirdUpload copies a locally persisted file to the requested object store.
// Local and unknown source values keep the existing local-only behavior.
func thirdUpload(ctx context.Context, source, name, path string) error {
	switch source {
	case "2":
		return upload(ctx, filestore.Aliyun, config.ExtConfig.FileStore.AliYun, "img/"+name, path)
	case "3":
		return upload(ctx, filestore.Qiniu, config.ExtConfig.FileStore.QiNiu, "img/"+name, path)
	}
	return nil
}

func upload(ctx context.Context, driver filestore.Driver, store config.ObjectStore, name, path string) error {
	client, err := filestore.New(filestore.Config{
		Driver:          driver,
		Endpoint:        store.Endpoint,
		AccessKeyID:     store.AccessKeyID,
		AccessKeySecret: store.AccessKeySecret,
		BucketName:      store.BucketName,
	})
	if err != nil {
		return err
	}
	return client.Upload(ctx, name, path)
}

type uploadBatch struct {
	source, tag, urlPrefix string
	saved                  []string
}

func (u *uploadBatch) persist(name, extension string, save func(string) error) (FileResponse, error) {
	filename := path + uuid.New().String() + extension
	if err := os.MkdirAll(path, 0750); err != nil {
		return FileResponse{}, err
	}
	u.saved = append(u.saved, filename)
	if err := save(filename); err != nil {
		return FileResponse{}, err
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return FileResponse{}, err
	}
	contentType, err := uploadContentType(filename)
	if err != nil {
		return FileResponse{}, err
	}
	// Preserve the bundled UI's URL contract: single uploads and non-local
	// multi/base64 responses are relative; source=1 multi/base64 are absolute.
	responsePath, fullPath := "/"+filename, "/"+filename
	if (u.tag == "2" || u.tag == "3") && u.source == "1" {
		responsePath, fullPath = filename, u.urlPrefix+"/"+filename
	}
	return FileResponse{Size: stat.Size(), Path: responsePath, FullPath: fullPath, Name: name, Type: contentType}, nil
}
func (u *uploadBatch) save(c *gin.Context) (any, error) {
	var err error
	var result any
	if u.tag == "3" {
		var data []byte
		var extension string
		data, extension, err = decodeUploadImage(c.PostForm("file"))
		if err == nil {
			result, err = u.persist("", extension, func(filename string) error { return os.WriteFile(filename, data, 0600) })
		}
	} else {
		var files []*multipart.FileHeader
		if c.Request.MultipartForm != nil {
			files = c.Request.MultipartForm.File["file"]
		}
		if len(files) == 0 || (u.tag != "2" && len(files) != 1) {
			err = fmt.Errorf("%w: choose the expected number of files", invalidUploadErr)
		} else {
			results := make([]FileResponse, 0, len(files))
			for _, file := range files {
				var item FileResponse
				item, err = u.persist(file.Filename, filepath.Ext(file.Filename), func(filename string) error { return c.SaveUploadedFile(file, filename) })
				if err != nil {
					break
				}
				results = append(results, item)
			}
			if err == nil {
				if u.tag == "2" {
					result = results
				} else {
					result = results[0]
				}
			}
		}
	}
	return result, err
}

func (u *uploadBatch) discard(warn func(error)) {
	for _, name := range u.saved {
		if removeErr := os.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			warn(removeErr)
		}
	}
}
