package envconfig

import (
	"context"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

type item struct {
	Key     string
	EnvKeys []string
}

var items = []item{
	{Key: "system.appName", EnvKeys: []string{"GF_SYSTEM_APPNAME", "YOUBAN_APP_NAME", "APP_NAME"}},
	{Key: "system.mode", EnvKeys: []string{"GF_SYSTEM_MODE", "YOUBAN_MODE", "APP_MODE", "MODE"}},
	{Key: "system.debug", EnvKeys: []string{"GF_SYSTEM_DEBUG", "YOUBAN_DEBUG", "APP_DEBUG", "DEBUG"}},
	{Key: "server.address", EnvKeys: []string{"GF_SERVER_ADDRESS", "YOUBAN_SERVER_ADDRESS"}},
	{Key: "cache.adapter", EnvKeys: []string{"GF_CACHE_ADAPTER", "YOUBAN_CACHE_ADAPTER", "CACHE_ADAPTER"}},
	{Key: "token.secretKey", EnvKeys: []string{"GF_TOKEN_SECRETKEY", "GF_TOKEN_SECRET_KEY", "YOUBAN_TOKEN_SECRET", "TOKEN_SECRET_KEY"}},
	{Key: "queue.driver", EnvKeys: []string{"GF_QUEUE_DRIVER", "YOUBAN_QUEUE_DRIVER", "QUEUE_DRIVER"}},
	{Key: "redis.default.address", EnvKeys: []string{"GF_REDIS_DEFAULT_ADDRESS", "YOUBAN_REDIS_ADDRESS", "REDIS_ADDRESS"}},
	{Key: "redis.default.db", EnvKeys: []string{"GF_REDIS_DEFAULT_DB", "YOUBAN_REDIS_DB", "REDIS_DB"}},
	{Key: "redis.default.pass", EnvKeys: []string{"GF_REDIS_DEFAULT_PASS", "YOUBAN_REDIS_PASS", "YOUBAN_REDIS_PASSWORD", "REDIS_PASS"}},
	{Key: "database.default.link", EnvKeys: []string{"GF_DATABASE_DEFAULT_LINK", "YOUBAN_DATABASE_LINK", "DATABASE_DEFAULT_LINK"}},
	{Key: "database.default.debug", EnvKeys: []string{"GF_DATABASE_DEFAULT_DEBUG", "YOUBAN_DB_DEBUG", "DATABASE_DEBUG"}},
	{Key: "database.default.Prefix", EnvKeys: []string{"GF_DATABASE_DEFAULT_PREFIX", "YOUBAN_DB_PREFIX", "DATABASE_PREFIX"}},
	{Key: "runtime.roles", EnvKeys: []string{"YOUBAN_RUNTIME_ROLES"}},
	{Key: "telemetry.switch", EnvKeys: []string{"YOUBAN_TELEMETRY_SWITCH"}},
	{Key: "telemetry.endpoint", EnvKeys: []string{"YOUBAN_TELEMETRY_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"}},
	{Key: "telemetry.secure", EnvKeys: []string{"YOUBAN_TELEMETRY_SECURE"}},
	{Key: "telemetry.serviceName", EnvKeys: []string{"YOUBAN_TELEMETRY_SERVICE_NAME", "OTEL_SERVICE_NAME"}},
	{Key: "telemetry.sampleRatio", EnvKeys: []string{"YOUBAN_TELEMETRY_SAMPLE_RATIO"}},
	{Key: "telemetry.metricsIntervalSeconds", EnvKeys: []string{"YOUBAN_TELEMETRY_METRICS_INTERVAL_SECONDS"}},
	{Key: "youbanPublish.queue.concurrency", EnvKeys: []string{"YOUBAN_PUBLISH_QUEUE_CONCURRENCY"}},
	{Key: "youbanTgBotGateway.queue.concurrency", EnvKeys: []string{"YOUBAN_TG_BOT_GATEWAY_QUEUE_CONCURRENCY"}},
	{Key: "youbanPublish.queue.backgroundConcurrency", EnvKeys: []string{"YOUBAN_PUBLISH_BACKGROUND_CONCURRENCY"}},
	{Key: "youbanPublish.queue.accountBusyTimeoutSeconds", EnvKeys: []string{"YOUBAN_PUBLISH_ACCOUNT_BUSY_TIMEOUT_SECONDS"}},
	{Key: "youbanPublish.queue.mediaConcurrency", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_WORKER_CONCURRENCY"}},
	{Key: "youbanPublish.queue.mediaBulkShards", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_BULK_SHARDS"}},
	{Key: "youbanPublish.queue.mediaRealtimeWeight", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_REALTIME_WEIGHT"}},
	{Key: "youbanPublish.queue.mediaBulkWeight", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_BULK_WEIGHT"}},
	{Key: "youbanPublish.queue.mediaLegacyWeight", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_LEGACY_WEIGHT"}},
	{Key: "youbanPublish.fullPush.expandWorkerCount", EnvKeys: []string{"YOUBAN_PUBLISH_FULL_PUSH_EXPAND_WORKERS"}},
	{Key: "youbanPublish.fullPush.pageSize", EnvKeys: []string{"YOUBAN_PUBLISH_FULL_PUSH_PAGE_SIZE"}},
	{Key: "youbanPublish.fullPush.candidateCount", EnvKeys: []string{"YOUBAN_PUBLISH_FULL_PUSH_CANDIDATE_COUNT"}},
	{Key: "youbanPublish.fullPush.schedulerIntervalSeconds", EnvKeys: []string{"YOUBAN_PUBLISH_FULL_PUSH_INTERVAL_SECONDS"}},
	{Key: "youbanPublish.fullPush.expandLeaseSeconds", EnvKeys: []string{"YOUBAN_PUBLISH_FULL_PUSH_LEASE_SECONDS"}},
	{Key: "youbanPublish.collect.globalMediaConcurrency", EnvKeys: []string{"YOUBAN_PUBLISH_GLOBAL_MEDIA_CONCURRENCY"}},
	{Key: "youbanPublish.collect.accountMediaConcurrency", EnvKeys: []string{"YOUBAN_PUBLISH_ACCOUNT_MEDIA_CONCURRENCY"}},
	{Key: "youbanPublish.collect.accountMediaLeaseSeconds", EnvKeys: []string{"YOUBAN_PUBLISH_ACCOUNT_MEDIA_LEASE_SECONDS"}},
	{Key: "youbanPublish.collect.mediaFileConcurrency", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_FILE_CONCURRENCY"}},
	{Key: "youbanPublish.collect.mediaDownloadThreads", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_DOWNLOAD_THREADS"}},
	{Key: "youbanPublish.collect.mediaRecoveryBatchSize", EnvKeys: []string{"YOUBAN_PUBLISH_MEDIA_RECOVERY_BATCH_SIZE"}},
	{Key: "youbanPublish.collect.materialWindowBatchSize", EnvKeys: []string{"YOUBAN_PUBLISH_MATERIAL_WINDOW_BATCH_SIZE"}},
	{Key: "youbanPublish.collect.materialVerifyWindowSeconds", EnvKeys: []string{"YOUBAN_PUBLISH_MATERIAL_VERIFY_WINDOW_SECONDS"}},
	{Key: "telegramCollector.enabled", EnvKeys: []string{"YOUBAN_TELEGRAM_COLLECTOR_ENABLED"}},
	{Key: "telegramCollector.worker.concurrency", EnvKeys: []string{"YOUBAN_TELEGRAM_COLLECTOR_CONCURRENCY"}},
	{Key: "telegramCollector.worker.recoveryBatchSize", EnvKeys: []string{"YOUBAN_TELEGRAM_COLLECTOR_RECOVERY_BATCH_SIZE"}},
	{Key: "telegramCollector.delivery.concurrency", EnvKeys: []string{"YOUBAN_TELEGRAM_DELIVERY_CONCURRENCY"}},
	{Key: "telegramCollector.media.concurrency", EnvKeys: []string{"YOUBAN_TELEGRAM_MEDIA_CONCURRENCY"}},
	{Key: "telegramCollector.account.leaseSeconds", EnvKeys: []string{"YOUBAN_TELEGRAM_ACCOUNT_LEASE_SECONDS"}},
	{Key: "tcp.server.address", EnvKeys: []string{"GF_TCP_SERVER_ADDRESS", "YOUBAN_TCP_SERVER_ADDRESS", "TCP_SERVER_ADDRESS"}},
	{Key: "tcp.client.cron.address", EnvKeys: []string{"GF_TCP_CLIENT_CRON_ADDRESS", "YOUBAN_TCP_CRON_ADDRESS", "TCP_CRON_ADDRESS"}},
	{Key: "tcp.client.cron.group", EnvKeys: []string{"GF_TCP_CLIENT_CRON_GROUP", "YOUBAN_TCP_CRON_GROUP", "TCP_CRON_GROUP"}},
	{Key: "tcp.client.cron.name", EnvKeys: []string{"GF_TCP_CLIENT_CRON_NAME", "YOUBAN_TCP_CRON_NAME", "TCP_CRON_NAME"}},
	{Key: "tcp.client.cron.appId", EnvKeys: []string{"GF_TCP_CLIENT_CRON_APPID", "YOUBAN_TCP_CRON_APP_ID", "TCP_CRON_APP_ID"}},
	{Key: "tcp.client.cron.secretKey", EnvKeys: []string{"GF_TCP_CLIENT_CRON_SECRETKEY", "YOUBAN_TCP_CRON_SECRET_KEY", "TCP_CRON_SECRET_KEY"}},
	{Key: "tcp.client.auth.address", EnvKeys: []string{"GF_TCP_CLIENT_AUTH_ADDRESS", "YOUBAN_TCP_AUTH_ADDRESS", "TCP_AUTH_ADDRESS"}},
	{Key: "tcp.client.auth.group", EnvKeys: []string{"GF_TCP_CLIENT_AUTH_GROUP", "YOUBAN_TCP_AUTH_GROUP", "TCP_AUTH_GROUP"}},
	{Key: "tcp.client.auth.name", EnvKeys: []string{"GF_TCP_CLIENT_AUTH_NAME", "YOUBAN_TCP_AUTH_NAME", "TCP_AUTH_NAME"}},
	{Key: "tcp.client.auth.appId", EnvKeys: []string{"GF_TCP_CLIENT_AUTH_APPID", "YOUBAN_TCP_AUTH_APP_ID", "TCP_AUTH_APP_ID"}},
	{Key: "tcp.client.auth.secretKey", EnvKeys: []string{"GF_TCP_CLIENT_AUTH_SECRETKEY", "YOUBAN_TCP_AUTH_SECRET_KEY", "TCP_AUTH_SECRET_KEY"}},
	{Key: "content.cdnBaseUrl", EnvKeys: []string{"GF_CONTENT_CDNBASEURL", "GF_CONTENT_CDN_BASE_URL", "YOUBAN_CONTENT_CDN_BASE_URL", "CONTENT_CDN_BASE_URL"}},
	{Key: "youbanChat.pocketPing.baseUrl", EnvKeys: []string{"GF_YOUBANCHAT_POCKETPING_BASEURL", "YOUBAN_CHAT_POCKETPING_BASE_URL"}},
	{Key: "youbanChat.pocketPing.apiKey", EnvKeys: []string{"GF_YOUBANCHAT_POCKETPING_APIKEY", "YOUBAN_CHAT_POCKETPING_API_KEY"}},
	{Key: "youbanChat.telegram.chatId", EnvKeys: []string{"GF_YOUBANCHAT_TELEGRAM_CHATID", "YOUBAN_CHAT_TELEGRAM_CHAT_ID"}},
	{Key: "youbanChat.telegram.webhookBaseUrl", EnvKeys: []string{"GF_YOUBANCHAT_TELEGRAM_WEBHOOKBASEURL", "YOUBAN_CHAT_TELEGRAM_WEBHOOK_BASE_URL"}},
}

// Apply overlays selected runtime configuration from environment variables.
func Apply(ctx context.Context) {
	_ = ctx

	adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if !ok {
		return
	}

	applyDerived(adapter)
	for _, item := range items {
		if value, ok := firstEnv(item.EnvKeys...); ok {
			if item.Key == "database.default.link" && value == "" {
				continue
			}
			_ = adapter.Set(item.Key, value)
		}
	}
}

func applyDerived(adapter *gcfg.AdapterFile) {
	if value, ok := firstEnv("GF_SERVER_PORT", "YOUBAN_SERVER_PORT", "PORT"); ok && value != "" {
		_ = adapter.Set("server.address", ":"+strings.TrimPrefix(value, ":"))
	}

	if value, ok := firstEnv("YOUBAN_REDIS_HOST", "REDIS_HOST"); ok && value != "" {
		port := firstEnvOrDefault([]string{"YOUBAN_REDIS_PORT", "REDIS_PORT"}, "6379")
		_ = adapter.Set("redis.default.address", value+":"+port)
	}

	if value, ok := firstEnv("GF_DATABASE_DEFAULT_LINK", "YOUBAN_DATABASE_LINK", "DATABASE_DEFAULT_LINK"); !ok || value == "" {
		if link := buildDatabaseLinkFromEnv(); link != "" {
			_ = adapter.Set("database.default.link", link)
		}
	}
}

func buildDatabaseLinkFromEnv() string {
	driver := firstEnvOrDefault([]string{"YOUBAN_DB_DRIVER", "DB_DRIVER"}, "pgsql")
	host, ok := firstEnv("YOUBAN_DB_HOST", "DB_HOST")
	if !ok || host == "" {
		return ""
	}

	port := firstEnvOrDefault([]string{"YOUBAN_DB_PORT", "DB_PORT"}, defaultDatabasePort(driver))
	name := firstEnvOrDefault([]string{"YOUBAN_DB_NAME", "DB_NAME"}, "")
	user := firstEnvOrDefault([]string{"YOUBAN_DB_USER", "DB_USER"}, "")
	password := firstEnvOrDefault([]string{"YOUBAN_DB_PASSWORD", "DB_PASSWORD"}, "")

	if name == "" || user == "" {
		return ""
	}

	if driver == "mysql" {
		return "mysql:" + user + ":" + password + "@tcp(" + host + ":" + port + ")/" + name + "?loc=Local&parseTime=true&charset=utf8mb4"
	}
	return "pgsql:" + user + ":" + password + "@tcp(" + host + ":" + port + ")/" + name
}

func defaultDatabasePort(driver string) string {
	if driver == "mysql" {
		return "3306"
	}
	return "5432"
}

func firstEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			return value, true
		}
	}
	return "", false
}

func firstEnvOrDefault(keys []string, def string) string {
	if value, ok := firstEnv(keys...); ok {
		return value
	}
	return def
}
