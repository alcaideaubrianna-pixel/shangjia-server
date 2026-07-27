package sys

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

func collectPublishClientRequestId(event gdb.Record, rule gdb.Record) string {
	return fmt.Sprintf("collect:%s:%d", event["source_unique_key"].String(), rule["id"].Int64())
}

func (s *sSysPublish) markCollectEvent(ctx context.Context, id int64, status string, message string) error {
	_, err := pdao.YoubanPublishCollectEvent.Ctx(ctx).Where("id", id).Data(g.Map{
		"status":        status,
		"error_message": message,
		"processed_at":  gtime.Now(),
		"updated_at":    gtime.Now(),
	}).Update()
	return err
}

func collectTitle(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
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
