package sys

import (
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	trendDateLayout  = "2006-01-02"
	trendDefaultDays = 7
	trendMaxDays     = 90
)

type trendDateRange struct {
	Start string
	End   string
	Days  int
}

func resolveTrendDateRange(in *sysin.TrendInp) (*trendDateRange, error) {
	startDate := ""
	endDate := ""
	if in != nil {
		startDate = strings.TrimSpace(in.StartDate)
		endDate = strings.TrimSpace(in.EndDate)
	}
	if startDate != "" || endDate != "" {
		return resolveExplicitTrendDateRange(startDate, endDate)
	}
	return resolveDaysTrendDateRange(in)
}

func resolveExplicitTrendDateRange(startDate string, endDate string) (*trendDateRange, error) {
	if startDate == "" || endDate == "" {
		return nil, gerror.New("开始日期和结束日期必须同时填写")
	}
	start, err := parseTrendDate(startDate, "开始日期")
	if err != nil {
		return nil, err
	}
	end, err := parseTrendDate(endDate, "结束日期")
	if err != nil {
		return nil, err
	}
	if start.After(end) {
		return nil, gerror.New("开始日期不能晚于结束日期")
	}
	days := int(end.Sub(start).Hours()/24) + 1
	if days > trendMaxDays {
		return nil, gerror.New("趋势日期范围不能超过90天")
	}
	return &trendDateRange{Start: startDate, End: endDate, Days: days}, nil
}

func resolveDaysTrendDateRange(in *sysin.TrendInp) (*trendDateRange, error) {
	days := trendDefaultDays
	if in != nil && in.Days > 0 {
		days = in.Days
	}
	if days > trendMaxDays {
		return nil, gerror.New("趋势天数不能超过90天")
	}
	now := time.Now()
	start := now.AddDate(0, 0, -days+1).Format(trendDateLayout)
	end := now.Format(trendDateLayout)
	return &trendDateRange{Start: start, End: end, Days: days}, nil
}

func normalizeTrendDays(in *sysin.TrendInp) int {
	days := trendDefaultDays
	if in != nil && in.Days > 0 {
		days = in.Days
	}
	if days > trendMaxDays {
		days = trendMaxDays
	}
	return days
}

func parseTrendDate(value string, label string) (time.Time, error) {
	date, err := time.ParseInLocation(trendDateLayout, value, time.Local)
	if err != nil {
		return time.Time{}, gerror.New(label + "格式不合法，请使用YYYY-MM-DD")
	}
	if date.Format(trendDateLayout) != value {
		return time.Time{}, gerror.New(label + "格式不合法，请使用YYYY-MM-DD")
	}
	return date, nil
}
