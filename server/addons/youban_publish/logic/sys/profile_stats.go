package sys

import (
	"context"
	"fmt"

	"github.com/gogf/gf/v2/errors/gerror"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	basesysin "hotgo/internal/model/input/sysin"
	"hotgo/internal/service"
)

func (s *sSysPublish) cityForward(ctx context.Context, in *sysin.CityForwardInp, tenantId int64, accountId int64) (res *sysin.CityForwardModel, err error) {
	if in == nil {
		in = &sysin.CityForwardInp{}
	}
	data, err := service.SysProvinces().Select(ctx, &basesysin.ProvincesSelectInp{
		DataType: "pc",
		Value:    in.ParentId,
	})
	if err != nil {
		return nil, err
	}
	if data == nil {
		return &sysin.CityForwardModel{List: []*sysin.CityOptionModel{}}, nil
	}
	res = &sysin.CityForwardModel{List: make([]*sysin.CityOptionModel, 0, len(data.List))}
	for _, item := range data.List {
		res.List = append(res.List, &sysin.CityOptionModel{
			IsLeaf: item.IsLeaf,
			Label:  item.Label,
			Level:  item.Level,
			Value:  item.Value,
		})
	}
	return res, nil
}

func (s *sSysPublish) profileStats(ctx context.Context, in *sysin.TrendInp, tenantId int64, accountId int64) (res *sysin.ProfileStatsModel, err error) {
	dateRange, err := resolveTrendDateRange(in)
	if err != nil {
		return nil, err
	}
	res = &sysin.ProfileStatsModel{Trend: make([]*sysin.TrendPointModel, 0, dateRange.Days)}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	summaryFields := fmt.Sprintf(
		"COUNT(*) AS total,COALESCE(SUM(CASE WHEN p.status = 1 THEN 1 ELSE 0 END), 0) AS up_count,COALESCE(SUM(CASE WHEN p.status = 2 THEN 1 ELSE 0 END), 0) AS down_count,COALESCE(SUM(CASE WHEN p.review_status = '%s' THEN 1 ELSE 0 END), 0) AS pending_count,COALESCE(SUM(CASE WHEN p.review_status = '%s' THEN 1 ELSE 0 END), 0) AS approved_count,COALESCE(SUM(CASE WHEN p.review_status = '%s' THEN 1 ELSE 0 END), 0) AS rejected_count",
		consts.ContentReviewPending,
		consts.ContentReviewApproved,
		consts.ContentReviewRejected,
	)
	summary, err := base.Clone().Fields(summaryFields).One()
	if err != nil {
		return nil, gerror.Wrap(err, "统计资料失败")
	}
	res.Total = summary["total"].Int()
	res.UpCount = summary["up_count"].Int()
	res.DownCount = summary["down_count"].Int()
	res.Pending = summary["pending_count"].Int()
	res.Approved = summary["approved_count"].Int()
	res.Rejected = summary["rejected_count"].Int()
	var rows []struct {
		Date   string `json:"date"`
		Count  int    `json:"count"`
		Status int    `json:"status"`
	}
	trendMod, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return nil, err
	}
	if err = trendMod.Fields("DATE(p.created_at) AS date,p.status,COUNT(*) AS count").
		WhereGTE("p.created_at", dateRange.Start+" 00:00:00").
		WhereLTE("p.created_at", dateRange.End+" 23:59:59").
		Group("DATE(p.created_at),p.status").
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取资料趋势失败")
	}
	index := make(map[string]*sysin.TrendPointModel, dateRange.Days)
	start, _ := parseTrendDate(dateRange.Start, "开始日期")
	for i := 0; i < dateRange.Days; i++ {
		date := start.AddDate(0, 0, i).Format(trendDateLayout)
		point := &sysin.TrendPointModel{Date: date}
		index[date] = point
		res.Trend = append(res.Trend, point)
	}
	for _, row := range rows {
		point := index[row.Date]
		if point == nil {
			continue
		}
		point.ProfileCount += row.Count
		if row.Status == 1 {
			point.UpCount += row.Count
		}
		if row.Status == 2 {
			point.DownCount += row.Count
		}
	}
	return res, nil
}
