package sys

import (
	"os"
	"strings"
	"time"
)

const (
	tgJobRecoveryIntervalEnv        = "YOUBAN_PUBLISH_TG_RECOVERY_INTERVAL"
	tgOutboxScanIntervalEnv         = "YOUBAN_PUBLISH_TG_OUTBOX_SCAN_INTERVAL"
	tgSendWatchdogIntervalEnv       = "YOUBAN_PUBLISH_TG_WATCHDOG_INTERVAL"
	defaultTelegramRecoveryInterval = 5 * time.Minute
)

func telegramOutboxScanInterval() time.Duration {
	value := strings.TrimSpace(os.Getenv(tgOutboxScanIntervalEnv))
	if value == "" {
		return 30 * time.Second
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval < 10*time.Second {
		return 30 * time.Second
	}
	return interval
}

func telegramRecoveryInterval(envName string) time.Duration {
	value := strings.TrimSpace(os.Getenv(envName))
	if value == "" {
		return defaultTelegramRecoveryInterval
	}
	interval, err := time.ParseDuration(value)
	if err != nil || interval < 30*time.Second {
		return defaultTelegramRecoveryInterval
	}
	return interval
}
