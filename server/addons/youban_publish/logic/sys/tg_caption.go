package sys

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) telegramJobText(ctx context.Context, taskId int64) (string, error) {
	row, err := s.telegramJobTask(ctx, taskId)
	if err != nil {
		return "", err
	}
	setting, err := s.accountSetting(ctx, row["tenant_id"].Int64(), row["account_id"].Int64())
	if err != nil {
		return "", err
	}
	return buildTelegramTaskCaption(row, setting), nil
}

func buildTelegramTaskCaption(row gdb.Record, setting *sysin.AccountSettingModel) string {
	lines := make([]string, 0, 6)
	mark := telegramCaptionMark(row, setting)
	if setting != nil && setting.EnableTitleMark == 1 && setting.MarkPosition == "top" && mark != "" {
		lines = append(lines, mark)
	}
	if text := strings.TrimSpace(row["plain_text"].String()); text != "" {
		lines = append(lines, text)
	}
	if setting != nil && setting.EnableTitleMark == 1 && setting.MarkPosition != "top" && mark != "" {
		lines = appendCaptionMark(lines, mark, setting.MarkPosition)
	}
	if setting != nil && setting.EnableSuffix == 1 {
		if suffix := strings.TrimSpace(setting.SuffixContent); suffix != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, suffix)
		}
	}
	return strings.Join(lines, "\n")
}

func appendCaptionMark(lines []string, mark string, position string) []string {
	if position == "feeLine" {
		pattern := regexp.MustCompile(`(?i)(介绍费|介绍费用|服务费|费用)`)
		for index, line := range lines {
			if pattern.MatchString(line) {
				lines[index] = strings.TrimRight(line, " \t") + " " + mark
				return lines
			}
		}
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, mark)
}

func telegramCaptionMark(row gdb.Record, setting *sysin.AccountSettingModel) string {
	if setting == nil || setting.EnableTitleMark != 1 {
		return ""
	}
	if title := strings.TrimSpace(row["title"].String()); title != "" {
		return title
	}
	number := telegramCaptionNumber(row)
	if number == "" {
		return ""
	}
	if setting.NumberSource == "random" {
		return number
	}
	prefix := strings.TrimSpace(setting.CustomMarkText)
	if setting.MarkMode != "custom" || prefix == "" {
		prefix = strings.TrimSpace(row["account_nickname"].String())
	}
	if prefix == "" {
		return number
	}
	return fmt.Sprintf("%s%s", prefix, number)
}

func telegramCaptionNumber(row gdb.Record) string {
	if profileNo := strings.TrimSpace(row["profile_no"].String()); profileNo != "" {
		return profileNo
	}
	return ""
}
