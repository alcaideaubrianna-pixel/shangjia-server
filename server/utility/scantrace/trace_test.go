package scantrace

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

func TestFailureLogKeepsTraceAfterRequestCancellation(t *testing.T) {
	logger := g.Log()
	config := logger.GetConfig()
	t.Cleanup(func() { _ = logger.SetConfig(config) })
	var output bytes.Buffer
	logger.SetWriter(&output)
	logger.SetStdoutPrint(false)
	ctx, cancel := context.WithCancel(With(context.Background(), g.Map{"traceId": "40A80308", "messageId": 123}))
	background := context.WithoutCancel(ctx)
	cancel()
	finish := Begin(background, "album.finalize")
	finish(errors.New("读取扫图批次失败"))
	finish(nil)
	got := output.String()
	for _, want := range []string{`"traceId":"40A80308"`, `"messageId":123`, `"event":"start"`, `"event":"end"`, "读取扫图批次失败"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s in %s", want, got)
		}
	}
	if strings.Count(got, `"event":"end"`) != 1 {
		t.Fatalf("duplicate terminal log: %s", got)
	}
	if background.Err() != nil {
		t.Fatal("request cancellation stopped background finalizer")
	}
}

func TestContextIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	fields := g.Map{"traceId": "TCWYOTGH"}
	parent := With(ctx, fields)
	fields["traceId"] = "changed"
	child := With(parent, g.Map{"itemIndex": 1})
	if parent.Value(contextKey{}).(g.Map)["traceId"] != "TCWYOTGH" {
		t.Fatal("caller mutated diagnostic context")
	}
	if _, ok := parent.Value(contextKey{}).(g.Map)["itemIndex"]; ok {
		t.Fatal("child mutated parent")
	}
	if child.Value(contextKey{}).(g.Map)["traceId"] != "TCWYOTGH" {
		t.Fatal("trace lost")
	}
	a, _ := ctx.Deadline()
	b, _ := child.Deadline()
	if !a.Equal(b) {
		t.Fatal("deadline changed")
	}
	cancel()
	if child.Err() != context.Canceled {
		t.Fatal("cancellation lost")
	}
}

func TestSensitiveErrorRedaction(t *testing.T) {
	value := "Post https://api.telegram.org/bot123456789:abcdefghijklmnopqrstuvwxyz/sendVideo token 123456789:abcdefghijklmnopqrstuvwxyz"
	got := privateToken.ReplaceAllString(privateURL.ReplaceAllString(value, "[url]"), "[token]")
	if got != "Post [url] token [token]" {
		t.Fatalf("unsafe error: %s", got)
	}
}

func TestSummaryLogIncludesTraceCountsAndRedactsError(t *testing.T) {
	logger := g.Log()
	config := logger.GetConfig()
	t.Cleanup(func() { _ = logger.SetConfig(config) })
	var output bytes.Buffer
	logger.SetWriter(&output)
	logger.SetStdoutPrint(false)
	ctx := With(context.Background(), g.Map{"traceId": "40A80308", "requestId": "request-1"})
	Record(ctx, "search.summary", g.Map{"scopeAccounts": 503, "identityHits": 22, "incomplete": true}, errors.New("download https://secret.example/file?signature=private token 123456789:abcdefghijklmnopqrstuvwxyz"))
	got := output.String()
	for _, want := range []string{`"traceId":"40A80308"`, `"requestId":"request-1"`, `"scopeAccounts":503`, `"identityHits":22`, `"incomplete":true`, `"event":"summary"`, "[url]", "[token]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %s: %s", want, got)
		}
	}
	for _, secret := range []string{"signature=private", "secret.example", "abcdefghijklmnopqrstuvwxyz"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sensitive value logged: %s", secret)
		}
	}
}
