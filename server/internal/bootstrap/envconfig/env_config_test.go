package envconfig

import "testing"

func TestMediaFileCacheEnvironmentMappings(t *testing.T) {
	want := map[string]string{
		"youbanPublish.mediaFileCache.cosDownloadMode":    "YOUBAN_PUBLISH_MEDIA_CACHE_DOWNLOAD_MODE",
		"youbanPublish.mediaFileCache.workerCdnBaseUrl":   "YOUBAN_PUBLISH_MEDIA_CACHE_WORKER_CDN_BASE_URL",
		"youbanPublish.mediaFileCache.fallbackCdnBaseUrl": "YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL",
	}
	for _, item := range items {
		envKey, ok := want[item.Key]
		if !ok {
			continue
		}
		for _, candidate := range item.EnvKeys {
			if candidate == envKey {
				delete(want, item.Key)
				break
			}
		}
	}
	for key, envKey := range want {
		t.Errorf("missing environment mapping %s -> %s", envKey, key)
	}
}
