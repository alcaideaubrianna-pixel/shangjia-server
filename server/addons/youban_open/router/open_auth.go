package router

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gogf/gf/v2/errors/gcode"
	"github.com/gogf/gf/v2/net/ghttp"
	"hotgo/addons/youban_open/internal/opencontext"
	"hotgo/addons/youban_open/service"
	"hotgo/internal/library/response"
	"strconv"
	"strings"
	"time"
)

func openAPIAuth(r *ghttp.Request) {
	id := strings.TrimSpace(r.GetHeader("x-app-id"))
	secret, e := service.OpenAccess().AppSecret(r.Context(), id)
	if e != nil || secret == "" {
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "开放接口凭证无效")
		return
	}
	ts := r.GetHeader("x-timestamp")
	n, _ := strconv.ParseInt(ts, 10, 64)
	if n == 0 || time.Since(time.UnixMilli(n)) > 5*time.Minute {
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "开放接口请求已过期")
		return
	}
	nonce := r.GetHeader("x-nonce")
	sig := r.GetHeader("x-signature")
	bodyDigest := sha256.Sum256(r.GetBody())
	canonical := strings.ToUpper(r.Method) + "\n" + r.URL.Path + "\n" + r.URL.Query().Encode() + "\n" + ts + "\n" + nonce + "\n" + hex.EncodeToString(bodyDigest[:])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	provided, err := hex.DecodeString(sig)
	if err != nil || !hmac.Equal(mac.Sum(nil), provided) {
		response.JsonExit(r, gcode.CodeNotAuthorized.Code(), "开放接口签名无效")
		return
	}
	r.SetCtx(opencontext.WithAppId(r.Context(), id))
	r.Middleware.Next()
}
