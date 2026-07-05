package global

import (
	"context"
	"os"
	"strings"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gcfg"
)

type envConfigItem struct {
	Key     string
	EnvKeys []string
}

var envConfigItems = []envConfigItem{
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
	{Key: "content.cdnBaseUrl", EnvKeys: []string{"GF_CONTENT_CDNBASEURL", "GF_CONTENT_CDN_BASE_URL", "YOUBAN_CONTENT_CDN_BASE_URL", "CONTENT_CDN_BASE_URL"}},
	{Key: "youbanChat.pocketPing.baseUrl", EnvKeys: []string{"GF_YOUBANCHAT_POCKETPING_BASEURL", "YOUBAN_CHAT_POCKETPING_BASE_URL"}},
	{Key: "youbanChat.pocketPing.apiKey", EnvKeys: []string{"GF_YOUBANCHAT_POCKETPING_APIKEY", "YOUBAN_CHAT_POCKETPING_API_KEY"}},
	{Key: "youbanChat.telegram.chatId", EnvKeys: []string{"GF_YOUBANCHAT_TELEGRAM_CHATID", "YOUBAN_CHAT_TELEGRAM_CHAT_ID"}},
	{Key: "youbanChat.telegram.webhookBaseUrl", EnvKeys: []string{"GF_YOUBANCHAT_TELEGRAM_WEBHOOKBASEURL", "YOUBAN_CHAT_TELEGRAM_WEBHOOK_BASE_URL"}},
}

// ApplyEnvConfig overlays selected runtime configuration from environment variables.
func ApplyEnvConfig(ctx context.Context) {
	_ = ctx

	adapter, ok := g.Cfg().GetAdapter().(*gcfg.AdapterFile)
	if !ok {
		return
	}

	applyDerivedEnv(adapter)
	for _, item := range envConfigItems {
		if value, ok := firstEnv(item.EnvKeys...); ok {
			if item.Key == "database.default.link" && value == "" {
				continue
			}
			_ = adapter.Set(item.Key, value)
		}
	}
}

func applyDerivedEnv(adapter *gcfg.AdapterFile) {
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
