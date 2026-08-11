package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

func BuildMediaFingerprint(md5 string, size int64, kind, mimeType string) string {
	value := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(md5)),
		fmt.Sprintf("%d", size),
		strings.ToLower(strings.TrimSpace(kind)),
		strings.ToLower(strings.TrimSpace(mimeType)),
	}, ":")
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
