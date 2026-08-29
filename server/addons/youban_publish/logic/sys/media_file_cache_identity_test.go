package sys

import "testing"

func TestMediaFileCacheKeyIgnoresPublishSpecificFields(t *testing.T) {
	const source = "https://img.example.com/hotgo/file/a.jpg?sign=first"
	first := mediaFileCacheKey(&telegramMediaItem{StoragePath: "hotgo/file/a.jpg", AssetHash: "anti-scan:first", TgFileId: "file-first"}, source)
	second := mediaFileCacheKey(&telegramMediaItem{StoragePath: "hotgo/file/a.jpg", AssetHash: "anti-scan:second", TgFileId: "file-second"}, "https://img.example.com/hotgo/file/a.jpg?sign=second")
	if first != second {
		t.Fatalf("同一原始媒体不应因发布任务字段或URL签名变化产生不同缓存Key: %s != %s", first, second)
	}
}

func TestMediaFileCacheKeySeparatesDifferentObjects(t *testing.T) {
	first := mediaFileCacheKey(nil, "https://img.example.com/hotgo/file/a.jpg")
	second := mediaFileCacheKey(nil, "https://img.example.com/hotgo/file/b.jpg")
	if first == second {
		t.Fatal("不同对象不应复用同一个缓存Key")
	}
}

func TestStableAttachmentCacheKey(t *testing.T) {
	first := stableMediaFileCacheKey("attachment:123")
	second := stableMediaFileCacheKey("attachment:123")
	if first != second || first == "" {
		t.Fatalf("附件缓存Key应稳定，得到 %q 和 %q", first, second)
	}
}
