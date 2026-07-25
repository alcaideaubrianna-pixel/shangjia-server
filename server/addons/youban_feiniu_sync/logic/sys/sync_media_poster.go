package sys

import (
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
)

func feiNiuImportedVideoPoster(row gdb.Record, mediaType string) (string, string) {
	if mediaType != "video" {
		return "", ""
	}
	previewURL := strings.TrimSpace(row["preview_uri"].String())
	if previewURL == "" {
		return "", ""
	}
	if feiNiuMediaURLIsVideo(previewURL) {
		return "", ""
	}
	storagePath := normalizeTelegramContentStoragePathLocal(previewURL)
	if storagePath == "" {
		return "", ""
	}
	return feiNiuImportedMediaURLByStoragePath(storagePath), storagePath
}

func feiNiuMediaURLIsVideo(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	switch strings.ToLower(filepath.Ext(parsed.Path)) {
	case ".3gp", ".avi", ".m4v", ".mkv", ".mov", ".mp4", ".mpeg", ".mpg", ".webm", ".wmv":
		return true
	default:
		return false
	}
}

func feiNiuImportedMediaURLByStoragePath(storagePath string) string {
	storagePath = strings.TrimSpace(storagePath)
	if storagePath == "" {
		return ""
	}
	cdnBase := feiniuMediaContentCDNBaseURL()
	if cdnBase != "" {
		return cdnBase + "/" + strings.TrimLeft(storagePath, "/")
	}
	return "/" + strings.TrimLeft(storagePath, "/")
}
