package sys

import "encoding/json"

func mustJsonEncode(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeInt64Array(value string) []int64 {
	var out []int64
	_ = json.Unmarshal([]byte(value), &out)
	if out == nil {
		return []int64{}
	}
	return out
}

func decodeStringArray(value string) []string {
	var out []string
	_ = json.Unmarshal([]byte(value), &out)
	if out == nil {
		return []string{}
	}
	return out
}
