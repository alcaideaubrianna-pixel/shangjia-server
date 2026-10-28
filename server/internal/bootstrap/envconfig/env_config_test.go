package envconfig

import (
	"context"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/util/gconv"
	"os"
	"path/filepath"
	"testing"
)

func TestClusterEnvironmentMapping(t *testing.T) {
	for _, item := range items {
		if item.Key != "system.isCluster" {
			continue
		}
		for _, envKey := range item.EnvKeys {
			if envKey == "YOUBAN_CLUSTER_ENABLED" {
				return
			}
		}
	}
	t.Fatal("missing YOUBAN_CLUSTER_ENABLED mapping")
}

func TestMediaFileCacheEnvironmentMappings(t *testing.T) {
	want := map[string]string{
		"youbanPublish.mediaSearch.batchApproximateEnabled":   "YOUBAN_PUBLISH_MEDIA_SEARCH_BATCH_APPROXIMATE_ENABLED",
		"youbanPublish.profileOriginEnabled":                  "YOUBAN_PUBLISH_PROFILE_ORIGIN_ENABLED",
		"youbanPublish.collect.duplicateMediaReplaceEnabled":  "YOUBAN_PUBLISH_DUPLICATE_MEDIA_REPLACE_ENABLED",
		"youbanPublish.collect.mediaAvailabilityCheckEnabled": "YOUBAN_PUBLISH_COLLECT_MEDIA_AVAILABILITY_CHECK_ENABLED",
		"youbanPublish.publish.mediaAvailabilityCheckEnabled": "YOUBAN_PUBLISH_PUBLISH_MEDIA_AVAILABILITY_CHECK_ENABLED",
		"youbanPublish.mediaFileCache.cosDownloadMode":        "YOUBAN_PUBLISH_MEDIA_CACHE_DOWNLOAD_MODE",
		"youbanPublish.mediaFileCache.workerCdnBaseUrl":       "YOUBAN_PUBLISH_MEDIA_CACHE_WORKER_CDN_BASE_URL",
		"youbanPublish.mediaFileCache.cosDownloadCdnBaseUrl":  "YOUBAN_PUBLISH_COS_DOWNLOAD_CDN_URL",
		"youbanPublish.mediaFileCache.fallbackCdnBaseUrl":     "YOUBAN_PUBLISH_MEDIA_CACHE_FALLBACK_CDN_BASE_URL",
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

func TestDatabasePoolEnvironmentOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("database:\n  default:\n    maxOpen: 20\n    maxIdle: 5\n    maxLifeTime: 10m\n"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter, err := gcfg.NewAdapterFile(path)
	if err != nil {
		t.Fatal(err)
	}
	previous := g.Cfg().GetAdapter()
	g.Cfg().SetAdapter(adapter)
	t.Cleanup(func() { g.Cfg().SetAdapter(previous) })
	for _, key := range []string{"GF_DATABASE_DEFAULT_MAXOPEN", "GF_DATABASE_DEFAULT_MAXIDLE", "GF_DATABASE_DEFAULT_MAXLIFETIME"} {
		t.Setenv(key, "")
	}
	t.Setenv("YOUBAN_DB_MAX_OPEN", "12")
	t.Setenv("YOUBAN_DB_MAX_IDLE", "3")
	t.Setenv("YOUBAN_DB_MAX_LIFETIME", "15m")
	// Preferred aliases must not be populated with empty strings: firstEnv intentionally treats them as explicit values.
	for _, key := range []string{"GF_DATABASE_DEFAULT_MAXOPEN", "GF_DATABASE_DEFAULT_MAXIDLE", "GF_DATABASE_DEFAULT_MAXLIFETIME"} {
		os.Unsetenv(key)
	}
	Apply(context.Background())
	for key, want := range map[string]string{"database.default.maxOpen": "12", "database.default.maxIdle": "3", "database.default.maxLifeTime": "15m"} {
		value, err := adapter.Get(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if gconv.String(value) != want {
			t.Fatalf("%s got %v want %s", key, value, want)
		}
	}
	t.Setenv("GF_DATABASE_DEFAULT_MAXOPEN", "14")
	Apply(context.Background())
	value, err := adapter.Get(context.Background(), "database.default.maxOpen")
	if err != nil {
		t.Fatal(err)
	}
	if gconv.Int(value) != 14 {
		t.Fatalf("preferred alias did not win: %v", value)
	}
}
