// Package dailyreport renders the same report for interactive browsing and notifications.
package dailyreport

import (
	"fmt"
	"html"
	"strings"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func Messages(item *sysin.DailyReportModel) []string {
	if item == nil {
		return nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "<b>📊 %s 推送报告</b>\n\n📥 采集资料：%d 组\n\n<b>来源频道：</b>\n", html.EscapeString(item.ReportDate), item.CollectedTotal)
	if len(item.Sources) == 0 {
		text.WriteString("今日暂无新增采集。\n")
	}
	for _, source := range item.Sources {
		if source != nil {
			fmt.Fprintf(&text, "• %s：采集 %d 组\n", Title(source.SourceTitle), source.Count)
		}
	}
	fmt.Fprintf(&text, "\n<b>采集推送</b>\n✅ 成功：%d 组\n❌ 失败：%d 组\n⏳ 处理中：%d 组\n\n<b>循环上架</b>\n✅ 成功：%d 组\n❌ 失败：%d 组\n⏳ 处理中：%d 组\n\n推送按频道投递次数统计，同一资料发往多个频道会分别计数。", item.PublishSuccess, item.PublishFailed, item.PublishProcessing, item.CycleSuccess, item.CycleFailed, item.CycleProcessing)
	for _, channel := range item.Channels {
		if channel != nil {
			kind := "采集推送"
			if channel.Action == "cycle_publish" {
				kind = "循环上架"
			}
			fmt.Fprintf(&text, "\n• [%s] %s：成功%d，失败%d，处理中%d", Title(channel.Title), kind, channel.Success, channel.Failed, channel.Processing)
		}
	}
	return Parts(text.String())
}

// Parts keeps HTML tags and entities intact by splitting only at complete lines.
func Parts(text string) []string {
	const limit = 3000
	parts := []string{}
	current := ""
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(current))+len([]rune(line))+1 > limit && current != "" {
			parts = append(parts, current)
			current = ""
		}
		if current != "" {
			current += "\n"
		}
		current += line
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
func Title(title string) string {
	runes := []rune(title)
	if len(runes) > 200 {
		title = string(runes[:200]) + "…"
	}
	return html.EscapeString(title)
}
