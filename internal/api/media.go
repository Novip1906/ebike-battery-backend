package api

import (
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	maxImageSize = 10 << 20
	maxVideoSize = 50 << 20
)

var (
	imageExtensions = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}
	videoExtensions = map[string]string{"video/mp4": ".mp4"}
)

type mediaFile struct {
	header      *multipart.FileHeader
	contentType string
	extension   string
}

type mediaRule struct {
	field      string
	maxSize    int64
	extensions map[string]string
	tooBig     string
	wrongType  string
}

var (
	imageRule = mediaRule{
		field: "image", maxSize: maxImageSize, extensions: imageExtensions,
		tooBig:    "Фото режима не больше 10 МБ.",
		wrongType: "Фото режима должно быть изображением JPEG, PNG или WebP.",
	}
	videoRule = mediaRule{
		field: "video", maxSize: maxVideoSize, extensions: videoExtensions,
		tooBig:    "Видео режима не больше 50 МБ.",
		wrongType: "Видео режима должно быть файлом MP4.",
	}
)

type mediaError struct {
	status  int
	message string
}

func (e mediaError) Error() string {
	return e.message
}

func readMediaFile(c *gin.Context, rule mediaRule) (*mediaFile, error) {
	header, err := c.FormFile(rule.field)
	if errors.Is(err, http.ErrMissingFile) {
		if _, sentAsText := c.Request.MultipartForm.Value[rule.field]; sentAsText {
			return nil, mediaError{status: http.StatusBadRequest, message: fmt.Sprintf("Поле %s должно быть файлом, а не текстом.", rule.field)}
		}
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if header.Size > rule.maxSize {
		return nil, mediaError{status: http.StatusRequestEntityTooLarge, message: rule.tooBig}
	}
	contentType, err := sniffContentType(header)
	if err != nil {
		return nil, err
	}
	extension, ok := rule.extensions[contentType]
	if !ok {
		return nil, mediaError{status: http.StatusBadRequest, message: rule.wrongType}
	}
	return &mediaFile{header: header, contentType: contentType, extension: extension}, nil
}

func sniffContentType(header *multipart.FileHeader) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	buffer := make([]byte, 512)
	read, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	return http.DetectContentType(buffer[:read]), nil
}
