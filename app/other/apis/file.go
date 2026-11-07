package apis

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/go-admin-team/go-admin-core/v2/sdk/api"
	"github.com/google/uuid"
	"go-admin/common/file_store"
	"go-admin/config"
	_ "golang.org/x/image/webp"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type FileResponse struct {
	Size     int64  `json:"size"`
	Path     string `json:"path"`
	FullPath string `json:"full_path"`
	Name     string `json:"name"`
	Type     string `json:"type"`
}

const path = "static/uploadfile/"

type File struct {
	api.Api
}

const maxUploadRequestBytes int64 = 32 << 20

var errInvalidUpload = errors.New("invalid upload")

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
func (e File) UploadFile(c *gin.Context) {
	e.MakeContext(c)
	if c.Request.ContentLength > maxUploadRequestBytes {
		e.Error(http.StatusRequestEntityTooLarge, errInvalidUpload, "上传内容超过32 MiB限制")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadRequestBytes)
	var err error
	if strings.HasPrefix(strings.ToLower(c.GetHeader("Content-Type")), "multipart/form-data") {
		err = c.Request.ParseMultipartForm(8 << 20)
		if c.Request.MultipartForm != nil {
			defer c.Request.MultipartForm.RemoveAll()
		}
	} else {
		err = c.Request.ParseForm()
	}
	if err != nil {
		code := http.StatusUnprocessableEntity
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
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
	var saved []string
	succeeded := false
	defer func() {
		if !succeeded {
			for _, name := range saved {
				if removeErr := os.Remove(name); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
					e.Logger.Warnf("cleanup upload: %v", removeErr)
				}
			}
		}
	}()
	persist := func(name, extension string, save func(string) error) (FileResponse, error) {
		filename := path + uuid.New().String() + extension
		if err := os.MkdirAll(path, 0755); err != nil {
			return FileResponse{}, err
		}
		saved = append(saved, filename)
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
		if (tag == "2" || tag == "3") && source == "1" {
			responsePath, fullPath = filename, urlPrefix+"/"+filename
		}
		return FileResponse{Size: stat.Size(), Path: responsePath, FullPath: fullPath, Name: name, Type: contentType}, nil
	}
	var result interface{}
	if tag == "3" {
		var data []byte
		var extension string
		data, extension, err = decodeUploadImage(c.PostForm("file"))
		if err == nil {
			result, err = persist("", extension, func(filename string) error { return os.WriteFile(filename, data, 0644) })
		}
	} else {
		var files []*multipart.FileHeader
		if c.Request.MultipartForm != nil {
			files = c.Request.MultipartForm.File["file"]
		}
		if len(files) == 0 || (tag != "2" && len(files) != 1) {
			err = fmt.Errorf("%w: choose the expected number of files", errInvalidUpload)
		} else {
			results := make([]FileResponse, 0, len(files))
			for _, file := range files {
				var item FileResponse
				item, err = persist(file.Filename, filepath.Ext(file.Filename), func(filename string) error { return c.SaveUploadedFile(file, filename) })
				if err != nil {
					break
				}
				results = append(results, item)
			}
			if err == nil {
				if tag == "2" {
					result = results
				} else {
					result = results[0]
				}
			}
		}
	}
	if err == nil {
		for _, filename := range saved {
			if err = thirdUpload(source, filepath.Base(filename), filename); err != nil {
				break
			}
		}
	}
	if err != nil {
		code := 500
		if errors.Is(err, errInvalidUpload) {
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
		return nil, "", fmt.Errorf("%w: expected an image data URL", errInvalidUpload)
	}
	mediaType, _, err := mime.ParseMediaType(strings.TrimSuffix(strings.TrimPrefix(metadata, "data:"), ";base64"))
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid media type", errInvalidUpload)
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid base64", errInvalidUpload)
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid image", errInvalidUpload)
	}
	formats := map[string]string{"png": "image/png", "jpeg": "image/jpeg", "gif": "image/gif", "webp": "image/webp"}
	expected, ok := formats[format]
	if !ok || mediaType != expected {
		return nil, "", fmt.Errorf("%w: image media type does not match content", errInvalidUpload)
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

// thirdUpload copies the file that was already stored locally to the object
// store the request asked for. source "1", and anything unrecognised, keeps the
// local copy only.
//
// Both branches used to construct a zero-value ALiYunOSS and call UpLoad on it,
// which panicked - and the qiniu branch constructed the aliyun client, so
// source=3 never reached qiniu even in principle.
func thirdUpload(source string, name string, path string) error {
	switch source {
	case "2":
		return upload(file_store.AliYunOSS, config.ExtConfig.FileStore.AliYun, "img/"+name, path)
	case "3":
		return upload(file_store.QiNiuKodo, config.ExtConfig.FileStore.QiNiu, "img/"+name, path)
	}
	return nil
}

func upload(driver file_store.DriverType, store config.ObjectStore, name, path string) error {
	if !store.Configured() {
		return fmt.Errorf("file store %s is not configured; set it under extend.fileStore", driver)
	}
	oxs := file_store.OXS{
		Endpoint:        store.Endpoint,
		AccessKeyID:     store.AccessKeyID,
		AccessKeySecret: store.AccessKeySecret,
		BucketName:      store.BucketName,
	}
	client, err := oxs.Setup(driver)
	if err != nil {
		return err
	}
	return client.UpLoad(name, path)
}
