package file

import (
	"io"
	"mime/multipart"
	"os"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

// NewMultipartFileHeaderFromPath creates a multipart file header backed by a
// temporary file, so large media files do not need to be loaded into memory.
func NewMultipartFileHeaderFromPath(path string, filename string) (*multipart.FileHeader, func(), error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, gerror.New("文件路径不能为空")
	}
	input, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	temp, err := os.CreateTemp("", "hotgo-upload-")
	if err != nil {
		_ = input.Close()
		return nil, nil, err
	}
	cleanupTemp := func() {
		_ = input.Close()
		_ = temp.Close()
		_ = os.Remove(temp.Name())
	}

	writer := multipart.NewWriter(temp)
	part, err := writer.CreateFormFile("file", filename)
	if err == nil {
		_, err = io.Copy(part, input)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = input.Close()
	}
	if err != nil {
		cleanupTemp()
		return nil, nil, err
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		cleanupTemp()
		return nil, nil, err
	}

	form, err := multipart.NewReader(temp, writer.Boundary()).ReadForm(1)
	if err != nil {
		cleanupTemp()
		return nil, nil, err
	}
	files := form.File["file"]
	if len(files) == 0 {
		form.RemoveAll()
		cleanupTemp()
		return nil, nil, gerror.New("文件内容为空")
	}
	cleanup := func() {
		form.RemoveAll()
		cleanupTemp()
	}
	return files[0], cleanup, nil
}
