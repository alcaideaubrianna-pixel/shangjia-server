package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/model/input/sysin"
)

var collectGeneratedTitleNumberPattern = regexp.MustCompile(`(?im)^\s*编号\s*[:：=]\s*C\d+\s*$\n?`)

func collectPublishClientRequestId(event gdb.Record, rule gdb.Record) string {
	return fmt.Sprintf("collect:%s:%d", event["source_unique_key"].String(), rule["id"].Int64())
}

func (s *sSysPublish) markCollectEvent(ctx context.Context, id int64, status string, message string) error {
	data := g.Map{
		"status":        status,
		"error_message": message,
		"updated_at":    gtime.Now(),
	}
	if status == sysin.CollectEventStatusProcessed || status == sysin.CollectEventStatusIgnored || status == sysin.CollectEventStatusFailed {
		data["processed_at"] = gtime.Now()
	}
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", id).Data(data).Update()
	return err
}

func collectTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = collectGeneratedTitleNumberPattern.ReplaceAllString(text, "")
	if title, _, _ := materialImportTitle(text); title != "" {
		return title
	}
	runes := []rune(text)
	if len(runes) > 48 {
		return string(runes[:48])
	}
	return text
}

func collectHash(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}
