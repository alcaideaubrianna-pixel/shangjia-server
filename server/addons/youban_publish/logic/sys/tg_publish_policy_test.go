package sys

import (
	"testing"
	"time"

	"hotgo/addons/youban_publish/model"
)

func TestNormalizeTelegramPublishConfigDefaults(t *testing.T) {
	conf := normalizeTelegramPublishConfig(&model.PublishConfig{RetryEnabled: 1})
	if conf.SendIntervalSeconds != 5 || conf.RetryIntervalMinutes != 5 {
		t.Fatalf("unexpected defaults: %+v", conf)
	}
}

func TestConfigurableTelegramRetryDelayCapsAtTwoHours(t *testing.T) {
	if got := configurableTelegramRetryDelay(5*time.Minute, 1); got != 5*time.Minute {
		t.Fatalf("unexpected first retry delay: %v", got)
	}
	if got := configurableTelegramRetryDelay(60*time.Minute, 3); got != 2*time.Hour {
		t.Fatalf("retry delay must be capped: %v", got)
	}
}
