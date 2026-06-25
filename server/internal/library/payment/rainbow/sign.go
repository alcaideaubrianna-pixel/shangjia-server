// Package rainbow 彩虹易支付
package rainbow

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
)

func signParams(params map[string]string, key string) string {
	sum := md5.Sum([]byte(buildSignContent(params) + strings.TrimSpace(key)))
	return fmt.Sprintf("%x", sum)
}

func verifyParams(params map[string]string, key string) error {
	sign := params["sign"]
	if sign == "" {
		return gerror.New("彩虹易支付回调缺少签名")
	}
	if !strings.EqualFold(sign, signParams(params, key)) {
		return gerror.New("彩虹易支付验签不通过")
	}
	return nil
}

func buildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "sign" || key == "sign_type" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+params[key])
	}
	return strings.Join(parts, "&")
}
