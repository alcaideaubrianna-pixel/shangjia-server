package sys

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Only the branch prefix participates: group/message identities deliberately
// do not, so repeated material within one source/rule still deduplicates.
func collectProfileFingerprintNamespace(sourceKey string) string {
	parts := strings.SplitN(sourceKey, ":", 9)
	if len(parts) != 9 || parts[0] != "collect" || parts[1] != "v4" || parts[4] != "source" || parts[6] != "rule" || parts[8] == "" {
		return ""
	}
	for _, index := range []int{2, 3, 5, 7} {
		id, err := strconv.ParseInt(parts[index], 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[index] {
			return ""
		}
	}
	return strings.Join(parts[:8], ":")
}

// scopeCollectProfileFingerprints accepts RAW buildProfileFingerprints output.
// Always return a copy: callers retain raw signatures for legacy lookup. Apply
// once per generation boundary; this is not a rehash of stored fingerprints.
// Only exact index signatures change; media pHash bits are never modified.
func scopeCollectProfileFingerprints(sourceKey string, items []profileFingerprint) []profileFingerprint {
	if items == nil {
		return nil
	}
	result := make([]profileFingerprint, len(items))
	copy(result, items)
	namespace := collectProfileFingerprintNamespace(sourceKey)
	if namespace == "" {
		return result
	}
	for index := range result {
		digest := sha256.Sum256([]byte(namespace + "\x00" + result[index].Signature))
		// Fits the existing varchar(64), with no schema/index changes.
		result[index].Signature = hex.EncodeToString(digest[:])
	}
	return result
}
