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

// BuildMediaIdentityFingerprint returns a stable pre-download identity for a
// Telegram media object. Access hashes and file references are deliberately
// excluded because they are access credentials and file references can expire.
func BuildMediaIdentityFingerprint(fileUniqueID string, mediaID int64, dcID int, kind, variant string) string {
	identityType := "mtproto"
	if strings.TrimSpace(fileUniqueID) != "" {
		identityType = "bot"
	}
	value := strings.Join([]string{
		"identity", identityType, strings.ToLower(strings.TrimSpace(fileUniqueID)),
		fmt.Sprintf("%d", mediaID), fmt.Sprintf("%d", dcID),
		strings.ToLower(strings.TrimSpace(kind)), strings.ToLower(strings.TrimSpace(variant)),
	}, ":")
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
