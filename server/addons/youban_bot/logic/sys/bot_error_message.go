package sys

import (
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"hotgo/utility/sensitive"
)

func botErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	// Only show the outer business message, never the wrapped transport error.
	message := gerror.Current(err).Error()
	if sensitive.Diagnostic(message) != message || strings.Contains(message, "SQLSTATE") || strings.Contains(message, "read tcp ") {
		return "处理失败，服务暂时不可用，请稍后重试。"
	}
	return message
}
