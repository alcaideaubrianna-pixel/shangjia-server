package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
)

type profileMetadata struct {
	Province string
	City     string
	Tag      string
}

func (s *sSysPublish) enrichProfileMetadata(ctx context.Context, text string) (*profileMetadata, error) {
	province, city, err := materialImportRegionCodes(ctx, text)
	if err != nil {
		// Region codes are enrichment only. A temporary dictionary/database
		// failure must not discard an otherwise valid imported profile; the
		// original text is still persisted and can be enriched later.
		g.Log().Warning(ctx, "资料地区元数据解析失败，继续创建资料", g.Map{"error": err.Error()})
		province, city = "", ""
	}
	tag, err := s.materialImportMatchedTags(ctx, text)
	if err != nil {
		return nil, err
	}
	return &profileMetadata{
		Province: province,
		City:     city,
		Tag:      tag,
	}, nil
}
