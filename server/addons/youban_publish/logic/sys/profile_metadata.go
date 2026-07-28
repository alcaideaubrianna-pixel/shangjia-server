package sys

import "context"

type profileMetadata struct {
	Province string
	City     string
	Tag      string
}

func (s *sSysPublish) enrichProfileMetadata(ctx context.Context, text string) (*profileMetadata, error) {
	province, city, err := materialImportRegionCodes(ctx, text)
	if err != nil {
		return nil, err
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
