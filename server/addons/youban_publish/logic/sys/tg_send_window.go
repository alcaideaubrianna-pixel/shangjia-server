package sys

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

var errTelegramPublishWindowBlocked = errors.New("当前不在自动发送时间窗口内，等待下一个发送窗口")

func (s *sSysPublish) telegramPublishWindowDelay(ctx context.Context) (time.Duration, bool) {
	res, err := NewSysConfig().PublishConfigView(ctx, &sysin.PublishConfigViewInp{})
	if err != nil || res == nil || res.PublishConfig == nil {
		g.Log().Warningf(ctx, "读取TG发送时间窗口配置失败：%+v", err)
		return 0, false
	}
	conf := res.PublishConfig
	if conf.SendWindowEnabled != 1 {
		return 0, false
	}
	startMinute, ok := parseTelegramWindowMinute(conf.SendWindowStart)
	if !ok {
		g.Log().Warningf(ctx, "TG发送开始时间格式不合法：%s", conf.SendWindowStart)
		return 0, false
	}
	endMinute, ok := parseTelegramWindowMinute(conf.SendWindowEnd)
	if !ok {
		g.Log().Warningf(ctx, "TG发送结束时间格式不合法：%s", conf.SendWindowEnd)
		return 0, false
	}
	if startMinute == endMinute {
		g.Log().Warning(ctx, "TG发送时间窗口开始和结束时间不能相同")
		return 0, false
	}
	now := time.Now()
	if telegramMinuteInWindow(now.Hour()*60+now.Minute(), startMinute, endMinute) {
		return 0, true
	}
	return nextTelegramWindowDelay(now, startMinute), true
}

func parseTelegramWindowMinute(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, false
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func telegramMinuteInWindow(current int, start int, end int) bool {
	if start < end {
		return current >= start && current < end
	}
	return current >= start || current < end
}

func nextTelegramWindowDelay(now time.Time, startMinute int) time.Duration {
	target := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		startMinute/60,
		startMinute%60,
		0,
		0,
		now.Location(),
	)
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target.Sub(now)
}
