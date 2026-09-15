package sysin

import (
	"context"
	"testing"
)

func TestChannelSaveFilterAllowsDisablingStaleBatchConfig(t *testing.T) {
	in := &ChannelSaveInp{
		TgAccountId:         1,
		ChannelTitle:        "test",
		CyclePublishEnabled: 0,
		CyclePublishMode:    "batch",
		CycleBatchSize:      0,
		CycleBatchTime:      "",
		BotIds:              []int64{1},
	}

	if err := in.Filter(context.Background()); err != nil {
		t.Fatalf("关闭循环上架不应校验残留的批次参数: %v", err)
	}
	if in.CyclePublishMode != "time" || in.CycleBatchSize != 0 || in.CycleBatchTime != "" {
		t.Fatalf("关闭循环上架后参数未归一化: %+v", in)
	}
}

func TestChannelSaveFilterValidatesEnabledBatchConfig(t *testing.T) {
	in := &ChannelSaveInp{
		TgAccountId:         1,
		ChannelTitle:        "test",
		CyclePublishEnabled: 1,
		CyclePublishMode:    "batch",
		CycleBatchSize:      0,
		CycleBatchTime:      "",
		BotIds:              []int64{1},
	}

	if err := in.Filter(context.Background()); err == nil {
		t.Fatal("开启批次循环时应校验批次数量")
	}
}
