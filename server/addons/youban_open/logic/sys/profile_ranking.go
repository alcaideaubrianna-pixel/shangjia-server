package sys

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

type rankedProfileRow struct {
	ProfileId int64  `json:"profileId"`
	Province  string `json:"province"`
	HotScore  int64  `json:"hotScore"`
}
type preferredProvinceRow struct {
	Province string `json:"province"`
}

func (s *sOpenAccess) RankedProfileIds(ctx context.Context, appId, actorId, feed string, limit int) ([]int64, error) {
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	since := time.Now().AddDate(0, 0, -6).Format("2006-01-02")
	provinces := []string{}
	if strings.EqualFold(feed, "recommended") && strings.TrimSpace(actorId) != "" {
		var preferred []preferredProvinceRow
		err := g.DB().GetScan(ctx, &preferred, `SELECT p.province AS province
FROM hg_youban_open_profile_signal s
JOIN hg_content_profile p ON p.id=s.profile_id
WHERE s.app_id=? AND s.actor_id=? AND COALESCE(p.province,'')<>''
GROUP BY p.province
ORDER BY SUM(s.view_count + CASE WHEN s.is_favorite=1 THEN 5 ELSE 0 END) DESC
LIMIT 3`, appId, actorId)
		if err != nil {
			return nil, gerror.Wrap(err, "读取用户推荐偏好失败")
		}
		for _, row := range preferred {
			if row.Province != "" {
				provinces = append(provinces, row.Province)
			}
		}
	}

	model := g.DB().Model(dailyMetricTable+" m").Ctx(ctx).
		LeftJoin("hg_content_profile p", "p.id=m.profile_id").
		Fields("m.profile_id", "MAX(p.province) AS province", "SUM(m.favorite_count * 8 + m.unique_view_count * 3 + m.view_count) AS hot_score").
		Where("m.app_id", appId).
		WhereGTE("m.metric_date", since).
		Group("m.profile_id").
		OrderDesc("hot_score")
	var rows []rankedProfileRow
	if err := model.Limit(limit).Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "读取资料排名失败")
	}
	if len(provinces) > 0 {
		preferred := make(map[string]bool, len(provinces))
		for _, province := range provinces {
			preferred[province] = true
		}
		sort.SliceStable(rows, func(i, j int) bool {
			left, right := preferred[rows[i].Province], preferred[rows[j].Province]
			if left != right {
				return left
			}
			return rows[i].HotScore > rows[j].HotScore
		})
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row.ProfileId > 0 {
			ids = append(ids, row.ProfileId)
		}
	}
	return ids, nil
}
