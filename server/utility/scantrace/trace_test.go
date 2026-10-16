package scantrace

import (
	"context"
	"testing"
	"time"

	"github.com/gogf/gf/v2/frame/g"
)

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
