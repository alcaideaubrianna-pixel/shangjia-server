package sys

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Run explicitly with ONLINE_FEE_REPLAY_INPUT/OUTPUT. It is skipped during
// normal CI because the fixture is a local production-data artifact.
func TestOnlineFeeReplayFixture(t *testing.T) {
	inPath, outPath := os.Getenv("ONLINE_FEE_REPLAY_INPUT"), os.Getenv("ONLINE_FEE_REPLAY_OUTPUT")
	if inPath == "" || outPath == "" {
		t.Skip("ONLINE_FEE_REPLAY_INPUT/OUTPUT not set")
	}
	in, err := os.Open(inPath)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	writer := bufio.NewWriterSize(out, 1<<20)
	defer writer.Flush()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	count := 0
	for scanner.Scan() {
		var row struct {
			EventId       int64  `json:"eventId"`
			RawTextBase64 string `json:"rawTextBase64"`
			RawText       string `json:"rawText"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		raw := []byte(row.RawText)
		if row.RawTextBase64 != "" {
			raw, err = base64.StdEncoding.DecodeString(strings.ReplaceAll(row.RawTextBase64, `\n`, ""))
			if err != nil {
				t.Fatal(err)
			}
		}
		output := applyCollectIntroFeeSuffix(string(raw), string(raw), "测试频道")
		result := map[string]interface{}{"eventId": row.EventId, "rawText": string(raw), "outputText": output, "testSuffix": "测试频道"}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write(append(encoded, '\n')); err != nil {
			t.Fatal(err)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("replayed=%d", count)
}
