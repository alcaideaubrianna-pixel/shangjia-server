// Package gmpay GMPay 推荐接入
package gmpay

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
	sign := strings.TrimSpace(params["signature"])
	if sign == "" {
		sign = strings.TrimSpace(params["sign"])
	}
	if sign == "" {
		return gerror.New("GMPay 回调缺少签名")
	}
	if !strings.EqualFold(sign, signParams(params, key)) {
		return gerror.New("GMPay 验签不通过")
	}
	return nil
}

func buildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "signature" || key == "sign" || value == "" {
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
