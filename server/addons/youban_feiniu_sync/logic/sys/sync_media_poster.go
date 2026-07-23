package sys

import (
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
	storagePath := normalizeTelegramContentStoragePathLocal(previewURL)
	if storagePath == "" {
		return previewURL, ""
	}
	return feiNiuImportedMediaURLByStoragePath(storagePath), storagePath
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
