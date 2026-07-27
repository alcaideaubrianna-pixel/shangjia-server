package sys

import (
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/os/gtime"
)

func newTelegramOperationNo(prefix string, taskId int64) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = "publish"
	}
	return fmt.Sprintf("%s:%d:%d", prefix, taskId, gtime.Now().TimestampNano())
}
