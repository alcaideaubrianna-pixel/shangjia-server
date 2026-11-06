package sysin

import (
	"context"
	"testing"
)

// TestAccountSettingPushEngineValidation 验证账户灰度引擎只接受已定义的模式。
func TestAccountSettingPushEngineValidation(t *testing.T) {
	valid := AccountSettingSaveInp{AccountId: 1, PushEngine: "unified"}
	if err := valid.Filter(context.Background()); err != nil {
		t.Fatalf("unified engine rejected: %v", err)
	}
	stream := AccountSettingSaveInp{AccountId: 1, PushEngine: "unified_stream"}
	if err := stream.Filter(context.Background()); err != nil {
		t.Fatalf("stream engine rejected: %v", err)
	}
	legacy := AccountSettingSaveInp{AccountId: 1}
	if err := legacy.Filter(context.Background()); err != nil || legacy.PushEngine != "legacy" {
		t.Fatalf("default engine = %q, err=%v", legacy.PushEngine, err)
	}
	invalid := AccountSettingSaveInp{AccountId: 1, PushEngine: "experimental"}
	if invalid.Filter(context.Background()) == nil {
		t.Fatal("invalid engine accepted")
	}
}
