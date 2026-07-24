package sys

import (
	"context"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// ProcessRemoteMediaAssets handles remote media that cannot be opened as a
// local file. Video similarity uses the remote poster as its visual asset.
func (s *sSysPublish) ProcessRemoteMediaAssets(ctx context.Context, in *sysin.RemoteMediaAssetsInp) (*sysin.RemoteMediaAssetsModel, error) {
	res := &sysin.RemoteMediaAssetsModel{}
	if in == nil {
		return res, nil
	}
	mediaType := strings.ToLower(strings.TrimSpace(in.MediaType))
	source := strings.TrimSpace(in.FileURL)
	if mediaType == "video" {
		source = strings.TrimSpace(in.PosterURL)
	}
	if source == "" {
		return res, nil
	}
	if !strings.HasPrefix(source, "http://") && !strings.HasPrefix(source, "https://") {
		return res, nil
	}
	hash, err := cachedRemoteImagePHash(ctx, source)
	if err != nil {
		return nil, err
	}
	res.Processed = true
	res.PerceptualHash = hash
	return res, nil
}
