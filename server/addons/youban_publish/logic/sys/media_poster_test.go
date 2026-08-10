package sys

import (
	"context"
	"errors"
	"strings"
	"testing"

	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model"
)

func TestVideoPosterCommandErrorUsesOutputTail(t *testing.T) {
	output := []byte(strings.Repeat("banner", 100) + "actual ffmpeg error")
	detail := videoPosterCommandError(output, errors.New("exit status 1"))
	if !strings.HasSuffix(detail, "actual ffmpeg error") {
		t.Fatalf("detail=%q", detail)
	}
	if len([]rune(detail)) > 503 {
		t.Fatalf("detail too long: %d", len([]rune(detail)))
	}
}

func TestVideoPosterCommandErrorFallsBackToRunError(t *testing.T) {
	detail := videoPosterCommandError(nil, errors.New("signal: killed"))
	if detail != "signal: killed" {
		t.Fatalf("detail=%q", detail)
	}
}

func TestMediaPosterUploadContextAddsApiModule(t *testing.T) {
	ctx := mediaPosterUploadContext(context.Background())
	if got := contexts.GetModule(ctx); got != consts.AppApi {
		t.Fatalf("module=%q, want %q", got, consts.AppApi)
	}
}

func TestMediaPosterUploadContextPreservesRequestModule(t *testing.T) {
	ctx := context.WithValue(context.Background(), consts.ContextHTTPKey, &model.Context{Module: consts.AppAdmin})
	if got := contexts.GetModule(mediaPosterUploadContext(ctx)); got != consts.AppAdmin {
		t.Fatalf("module=%q, want %q", got, consts.AppAdmin)
	}
}
