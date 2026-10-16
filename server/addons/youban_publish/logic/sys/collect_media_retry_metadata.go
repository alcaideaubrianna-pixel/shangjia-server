package sys

import "encoding/json"

// Retry metadata can refresh source fields, but cannot replace the server's
// published-asset identity. Invalid input must not erase valid stored metadata.
func mergeCollectMediaRetryMetadata(stored, incoming string) string {
	var current, patch map[string]json.RawMessage
	if json.Unmarshal([]byte(incoming), &patch) != nil || patch == nil {
		return stored
	}
	if json.Unmarshal([]byte(stored), &current) != nil || current == nil {
		current = make(map[string]json.RawMessage)
	}
	identity := current["reused_published_media_id"]
	for key, value := range patch {
		current[key] = value
	}
	if len(identity) > 0 {
		current["reused_published_media_id"] = identity
	}
	merged, err := json.Marshal(current)
	if err != nil {
		return stored
	}
	return string(merged)
}
