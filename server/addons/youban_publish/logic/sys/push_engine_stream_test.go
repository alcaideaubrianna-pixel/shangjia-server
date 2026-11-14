package sys

import (
	"context"
	"testing"
)

// TestStreamEngineUsesUnifiedDeleteRouting 验证全量 Stream 模式也启用新引擎的独立删除通道。
func TestStreamEngineUsesUnifiedDeleteRouting(t *testing.T) {
	publish := NewSysPublish()
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", pushEngineUnifiedStream)
	if !publish.unifiedPushEngineEnabled(context.Background(), 0, 0) {
		t.Fatal("Stream engine must use unified delete routing")
	}
	t.Setenv("YOUBAN_PUBLISH_PUSH_ENGINE", "")
	if !publish.unifiedPushEngineEnabled(context.Background(), 0, 0) {
		t.Fatal("missing engine configuration must use the unified producer")
	}
}
