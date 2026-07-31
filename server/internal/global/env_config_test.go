package global

import (
	"testing"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
	"github.com/gogf/gf/v2/os/gctx"
	"hotgo/internal/bootstrap/envconfig"
)

func TestApplyEnvConfig(t *testing.T) {
	ctx := gctx.New()
	adapter := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	adapter.SetContent(`
system:
  appName: "hotgo"
  mode: "develop"
  debug: true
server:
  address: ":8000"
cache:
  adapter: "memory"
token:
  secretKey: "default"
queue:
  driver: "disk"
redis:
  default:
    address: "127.0.0.1:6379"
    db: "0"
    pass: ""
database:
  default:
    link: "pgsql:default:default@tcp(127.0.0.1:5432)/default"
    debug: true
    Prefix: "hg_"
`, "config.yaml")
	defer adapter.ClearContent()

	t.Setenv("YOUBAN_MODE", "product")
	t.Setenv("YOUBAN_DEBUG", "false")
	t.Setenv("YOUBAN_SERVER_PORT", "9000")
	t.Setenv("YOUBAN_TOKEN_SECRET", "token-test")
	t.Setenv("YOUBAN_CACHE_ADAPTER", "redis")
	t.Setenv("YOUBAN_QUEUE_DRIVER", "redis")
	t.Setenv("YOUBAN_REDIS_HOST", "redis-test")
	t.Setenv("YOUBAN_REDIS_PORT", "6380")
	t.Setenv("YOUBAN_REDIS_DB", "3")
	t.Setenv("YOUBAN_REDIS_PASSWORD", "redis-pass")
	t.Setenv("YOUBAN_DB_DRIVER", "pgsql")
	t.Setenv("YOUBAN_DB_HOST", "pg-test")
	t.Setenv("YOUBAN_DB_PORT", "5433")
	t.Setenv("YOUBAN_DB_NAME", "db-test")
	t.Setenv("YOUBAN_DB_USER", "user-test")
	t.Setenv("YOUBAN_DB_PASSWORD", "db-pass")
	t.Setenv("YOUBAN_DB_DEBUG", "false")
	t.Setenv("YOUBAN_PUBLISH_MEDIA_WORKER_CONCURRENCY", "8")
	t.Setenv("YOUBAN_PUBLISH_MEDIA_BULK_SHARDS", "16")
	t.Setenv("YOUBAN_PUBLISH_MEDIA_REALTIME_WEIGHT", "24")
	t.Setenv("YOUBAN_PUBLISH_ACCOUNT_MEDIA_CONCURRENCY", "2")
	t.Setenv("YOUBAN_PUBLISH_MEDIA_RECOVERY_BATCH_SIZE", "300")

	envconfig.Apply(ctx)

	assertCfg := func(key string, want string) {
		t.Helper()
		if got := g.Cfg().MustGet(ctx, key).String(); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}

	assertCfg("system.mode", "product")
	assertCfg("system.debug", "false")
	assertCfg("server.address", ":9000")
	assertCfg("token.secretKey", "token-test")
	assertCfg("cache.adapter", "redis")
	assertCfg("queue.driver", "redis")
	assertCfg("redis.default.address", "redis-test:6380")
	assertCfg("redis.default.db", "3")
	assertCfg("redis.default.pass", "redis-pass")
	assertCfg("database.default.link", "pgsql:user-test:db-pass@tcp(pg-test:5433)/db-test")
	assertCfg("database.default.debug", "false")
	assertCfg("youbanPublish.queue.mediaConcurrency", "8")
	assertCfg("youbanPublish.queue.mediaBulkShards", "16")
	assertCfg("youbanPublish.queue.mediaRealtimeWeight", "24")
	assertCfg("youbanPublish.collect.accountMediaConcurrency", "2")
	assertCfg("youbanPublish.collect.mediaRecoveryBatchSize", "300")
}
