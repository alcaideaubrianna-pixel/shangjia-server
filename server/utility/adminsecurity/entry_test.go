package adminsecurity

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
)

func testServer(t *testing.T, configure func(*ghttp.Server)) string {
	t.Helper()
	s := g.Server(fmt.Sprintf("admin-entry-%d", time.Now().UnixNano()))
	s.SetAddr("127.0.0.1:0")
	s.SetDumpRouterMap(false)
	s.SetLogStdout(false)
	configure(s)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Shutdown() })
	return fmt.Sprintf("http://127.0.0.1:%d", s.GetListenedPort())
}

func TestEntryHTTPStaticAndAPIBoundaries(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "admin"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"index.html": "admin-page", "app.js": "admin-asset"} {
		if err := os.WriteFile(filepath.Join(dir, "admin", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := &Entry{Path: "/safe_abcdefghijklmnop", Key: strings.Repeat("k", 32), Prefix: "/admin", IndexFile: filepath.Join(dir, "admin/index.html")}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	base := testServer(t, func(s *ghttp.Server) {
		s.SetServerRoot(dir)
		s.BindHookHandler("/*any", ghttp.HookBeforeServe, e.BeforeServe)
		s.BindHandler("/admin/site/accountLogin", func(r *ghttp.Request) { r.Response.Write("login-handler") })
		s.BindHandler("/api/member/accountLogin", func(r *ghttp.Request) { r.Response.Write("member-handler") })
		s.BindHandler("/api/youban_tg_bot_gateway/telegram/webhook", func(r *ghttp.Request) { r.Response.Write("webhook") })
		s.BindHandler("/readyz", func(r *ghttp.Request) { r.Response.Write("ready") })
	})
	request := func(p string, cookie *http.Cookie, want int, body string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, base+p, nil)
		if cookie != nil {
			req.AddCookie(cookie)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != want || (body != "" && !strings.Contains(string(b), body)) {
			t.Fatalf("%s: status=%d body=%s", p, res.StatusCode, b)
		}
		return res
	}
	for _, p := range []string{"/admin", "/admin/", "/admin/index.html", "/admin/app.js", "/admin/site/accountLogin", "/ADMIN/site/accountLogin", "/%61dmin/site/accountLogin", "/login"} {
		request(p, nil, 404, "")
	}
	res := request(e.Path, nil, 200, "admin-page")
	if res.Header.Get("Cache-Control") != "no-store" || res.Header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("entry may leak via cache or referrer")
	}
	cookies := res.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe entry cookie", cookies)
	}
	request("/admin/app.js", cookies[0], 200, "admin-asset")
	request("/admin/site/accountLogin", cookies[0], 200, "login-handler")
	request("/admin", cookies[0], 404, "")
	request("/admin/index.html", cookies[0], 404, "")
	request("/api/member/accountLogin", nil, 200, "member-handler")
	request("/api/youban_tg_bot_gateway/telegram/webhook", nil, 200, "webhook")
	request("/readyz", nil, 200, "ready")
	tampered := *cookies[0]
	tampered.Value += "x"
	request("/admin/app.js", &tampered, 404, "")
}

func TestEntrySignatureExpiryRotationAndValidation(t *testing.T) {
	now := time.Now()
	e := &Entry{Path: "/safe_abcdefghijklmnop", Key: strings.Repeat("a", 32), Prefix: "/admin"}
	r, _ := http.NewRequest("GET", "https://example.test/admin/site/login", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: e.cookieValue(now)})
	if !e.validCookie(r, now) {
		t.Fatal("valid cookie rejected")
	}
	if e.validCookie(r, now.Add(entryTTL)) {
		t.Fatal("expired cookie accepted")
	}
	e.Key = strings.Repeat("b", 32)
	if e.validCookie(r, now) {
		t.Fatal("old signing key accepted")
	}
	for _, p := range []string{"/admin", "/api/a", "//abcdefghijklmnop", "/abcdefghijklmnop?x=y", "/abcdefghijklmnop/.."} {
		e.Path = p
		if e.Validate() == nil {
			t.Fatalf("unsafe entry accepted: %s", p)
		}
	}
}

func TestLoginLimiterDoesNotTouchMemberOrWebhook(t *testing.T) {
	count := 0
	l := NewLoginLimiter("/admin")
	l.Reserve = func(context.Context, string, int, time.Duration) (bool, error) { count++; return false, nil }
	base := testServer(t, func(s *ghttp.Server) {
		s.BindMiddleware("/*any", l.Middleware)
		for _, p := range []string{"/admin/site/accountLogin", "/admin/site/mobileLogin", "/api/member/accountLogin", "/api/youban_tg_bot_gateway/telegram/webhook"} {
			s.BindHandler(p, func(r *ghttp.Request) { r.Response.Write("ok") })
		}
	})
	for _, tt := range []struct {
		path   string
		status int
	}{
		{"/admin/site/accountLogin", 429}, {"/admin/site/mobileLogin", 429},
		{"/api/member/accountLogin", 200}, {"/api/youban_tg_bot_gateway/telegram/webhook", 200},
	} {
		res, err := http.Post(base+tt.path, "application/json", strings.NewReader(`{"username":"admin","isLock":true}`))
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != tt.status {
			t.Fatal(tt.path, res.StatusCode)
		}
	}
	if count != 2 {
		t.Fatal("unexpected public API limit", count)
	}
}

func TestBuiltAdminAssetsBehindEntry(t *testing.T) {
	dist, err := filepath.Abs("../../../web/dist")
	if err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dist, "index.html")
	if _, err := os.Stat(index); err != nil {
		t.Skip("build web before checking packaged assets")
	}
	e := &Entry{Path: "/safe_abcdefghijklmnop", Key: strings.Repeat("k", 32), Prefix: "/admin", IndexFile: index}
	base := testServer(t, func(s *ghttp.Server) {
		s.AddStaticPath("/admin", dist)
		s.BindHookHandler("/*any", ghttp.HookBeforeServe, e.BeforeServe)
	})
	res, err := http.Get(base + e.Path)
	if err != nil {
		t.Fatal(err)
	}
	page, err := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if err != nil || res.StatusCode != 200 || len(res.Cookies()) != 1 {
		t.Fatal("built entry unavailable", res.StatusCode, err)
	}
	cookie := res.Cookies()[0]
	assets := regexp.MustCompile(`(?:src|href)="([^"]+)"`).FindAllSubmatch(page, -1)
	checked := 0
	for _, asset := range assets {
		url := string(asset[1])
		if !strings.HasPrefix(url, "/admin/") {
			continue
		}
		for _, authenticated := range []bool{false, true} {
			req, _ := http.NewRequest("GET", base+url, nil)
			if authenticated {
				req.AddCookie(cookie)
			}
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			want := 404
			if authenticated {
				want = 200
			}
			if response.StatusCode != want {
				t.Fatalf("built asset %s authenticated=%v status=%d", url, authenticated, response.StatusCode)
			}
		}
		checked++
	}
	if checked < 3 {
		t.Fatal("expected built script, stylesheet and runtime config", checked)
	}
}
