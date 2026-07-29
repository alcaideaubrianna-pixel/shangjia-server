package sys

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func processRemoteMediaAssets(ctx context.Context, in *sysin.MediaAssetsInp) (*sysin.MediaAssetsModel, error) {
	res := &sysin.MediaAssetsModel{}
	if in == nil {
		return res, nil
	}
	mediaType := strings.ToLower(strings.TrimSpace(in.MediaType))
	source := strings.TrimSpace(in.FileURL)
	if mediaType == "video" {
		return processRemoteVideoAssets(ctx, source, strings.TrimSpace(in.PosterURL))
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

func processRemoteVideoAssets(ctx context.Context, videoURL string, posterURL string) (*sysin.MediaAssetsModel, error) {
	res := &sysin.MediaAssetsModel{}
	videoURL = strings.TrimSpace(videoURL)
	posterURL = strings.TrimSpace(posterURL)
	if posterURL != "" && !remoteMediaURLIsVideo(posterURL) {
		hash, err := cachedRemoteImagePHashValue(ctx, posterURL)
		if err == nil {
			res.Processed = true
			res.PerceptualHash = remotePHashString(hash)
			return res, nil
		}
		if videoURL == "" {
			return nil, err
		}
	}
	if videoURL == "" {
		return res, nil
	}
	hash, err := cachedRemoteVideoPHashValue(ctx, videoURL)
	if err != nil {
		return nil, err
	}
	res.Processed = true
	res.PerceptualHash = remotePHashString(hash)
	return res, nil
}

func remoteMediaURLIsVideo(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	switch strings.ToLower(filepath.Ext(parsed.Path)) {
	case ".3gp", ".avi", ".m4v", ".mkv", ".mov", ".mp4", ".mpeg", ".mpg", ".webm", ".wmv":
		return true
	default:
		return false
	}
}
