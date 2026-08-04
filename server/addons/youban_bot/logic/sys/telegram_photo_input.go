package sys

import (
	"context"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

func (s *sSysBot) telegramPhotoInput(ctx context.Context, source string) (models.InputFile, io.Closer, error) {
	source = normalizePreviewMediaURL(source)
	if source == "" {
		return nil, nil, gerror.New("欢迎图片地址为空")
	}
	serverRoot := g.Cfg().MustGet(ctx, "server.serverRoot", "resource/public").String()
	if localPath := resolvePublicMediaPath(serverRoot, source); localPath != "" {
		file, err := os.Open(localPath)
		if err != nil {
			return nil, nil, gerror.Wrap(err, "打开欢迎图片失败")
		}
		return &models.InputFileUpload{Filename: filepath.Base(localPath), Data: file}, file, nil
	}
	imageURL := normalizePreviewMediaURL(s.absoluteMediaURL(ctx, source))
	if imageURL == "" {
		return nil, nil, gerror.New("欢迎图片地址不可用")
	}
	return &models.InputFileString{Data: imageURL}, nil, nil
}

func resolvePublicMediaPath(serverRoot string, source string) string {
	serverRoot = strings.TrimSpace(serverRoot)
	source = strings.TrimSpace(source)
	if serverRoot == "" || source == "" {
		return ""
	}
	root, err := filepath.Abs(filepath.Clean(filepath.FromSlash(serverRoot)))
	if err != nil {
		return ""
	}
	mediaPath := source
	if parsed, parseErr := url.Parse(source); parseErr == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		mediaPath = parsed.Path
	}
	mediaPath = filepath.Clean(filepath.FromSlash(mediaPath))
	if filepath.IsAbs(mediaPath) && pathWithinRoot(root, mediaPath) && regularFile(mediaPath) {
		return mediaPath
	}
	relativePath := strings.TrimLeft(filepath.ToSlash(mediaPath), "/")
	for _, prefix := range []string{"resource/public/", "public/"} {
		relativePath = strings.TrimPrefix(relativePath, prefix)
	}
	candidate := filepath.Join(root, filepath.FromSlash(relativePath))
	if !pathWithinRoot(root, candidate) || !regularFile(candidate) {
		return ""
	}
	return candidate
}

func pathWithinRoot(root string, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
