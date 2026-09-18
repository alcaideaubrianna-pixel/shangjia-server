package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/youban_open/model/input/sysin"
)

const (
	interactionEventTable = "hg_youban_open_profile_event"
	dailyMetricTable      = "hg_youban_open_profile_metric_daily"
	dailyActorTable       = "hg_youban_open_profile_actor_daily"
	actorSignalTable      = "hg_youban_open_profile_signal"
)

func (s *sOpenAccess) RecordInteraction(ctx context.Context, appId string, in *sysin.ProfileInteractionInp) (accepted bool, err error) {
	appId, in.EventId, in.ActorId, in.Type = strings.TrimSpace(appId), strings.TrimSpace(in.EventId), strings.TrimSpace(in.ActorId), strings.ToLower(strings.TrimSpace(in.Type))
	if appId == "" || in.ProfileId <= 0 {
		return false, gerror.New("互动事件参数无效")
	}
	allowed, err := s.profileAllowedForApp(ctx, appId, in.ProfileId)
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, gerror.New("资料不存在或不在当前应用授权范围内")
	}
	now, metricDate := gtime.Now(), time.Now().Format("2006-01-02")
	if isOpenPgsql() {
		accepted, err = recordPostgresInteraction(ctx, appId, in, metricDate, now)
		if err != nil {
			return false, gerror.Wrap(err, "记录资料互动失败")
		}
		return accepted, nil
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		inserted, insertErr := insertInteractionEvent(ctx, tx, appId, in, now)
		if insertErr != nil || !inserted {
			accepted = inserted
			return insertErr
		}
		accepted = true
		uniqueView := false
		if in.Type == "view" {
			uniqueView, err = insertDailyActor(ctx, tx, appId, in.ActorId, in.ProfileId, metricDate, now)
			if err != nil {
				return err
			}
		}
		if err = upsertDailyMetric(ctx, tx, appId, in.ProfileId, metricDate, in.Type, uniqueView, now); err != nil {
			return err
		}
		return upsertActorSignal(ctx, tx, appId, in, now)
	})
	if err != nil {
		return false, gerror.Wrap(err, "记录资料互动失败")
	}
	return accepted, nil
}

func (s *sOpenAccess) profileAllowedForApp(ctx context.Context, appId string, profileId int64) (bool, error) {
	value, err := g.DB().GetValue(ctx, `SELECT EXISTS (
		SELECT 1
		FROM hg_youban_publish_profile_state ps
		JOIN hg_youban_publish_cms_tenant_binding b ON b.tenant_id=ps.tenant_id
		WHERE ps.profile_id=? AND ps.deleted_at IS NULL AND b.app_id=? AND b.status=?
	)`, profileId, appId, sysin.CmsBindingApproved)
	return err == nil && value.Bool(), err
}

// recordPostgresInteraction applies all counters in one PostgreSQL round trip.
// The event CTE is the idempotency gate: duplicate event IDs update nothing.
func recordPostgresInteraction(ctx context.Context, appId string, in *sysin.ProfileInteractionInp, metricDate string, now *gtime.Time) (bool, error) {
	row, err := g.DB().GetOne(ctx, `WITH event AS (
		INSERT INTO hg_youban_open_profile_event
			(app_id,event_id,actor_id,profile_id,event_type,occurred_at,created_at)
		VALUES (?,?,?,?,?,?,?) ON CONFLICT (app_id,event_id) DO NOTHING RETURNING 1
	), actor AS (
		INSERT INTO hg_youban_open_profile_actor_daily
			(app_id,actor_id,profile_id,metric_date,created_at)
		SELECT ?,?,?,?,? FROM event WHERE ?='view'
		ON CONFLICT (app_id,actor_id,profile_id,metric_date) DO NOTHING RETURNING 1
	), metric AS (
		INSERT INTO hg_youban_open_profile_metric_daily
			(app_id,profile_id,metric_date,view_count,unique_view_count,favorite_count,created_at,updated_at)
		SELECT ?,?,?,CASE WHEN ?='view' THEN 1 ELSE 0 END,
			CASE WHEN EXISTS (SELECT 1 FROM actor) THEN 1 ELSE 0 END,
			CASE WHEN ?='favorite' THEN 1 ELSE 0 END,?,? FROM event
		ON CONFLICT (app_id,profile_id,metric_date) DO UPDATE SET
			view_count=hg_youban_open_profile_metric_daily.view_count+EXCLUDED.view_count,
			unique_view_count=hg_youban_open_profile_metric_daily.unique_view_count+EXCLUDED.unique_view_count,
			favorite_count=hg_youban_open_profile_metric_daily.favorite_count+EXCLUDED.favorite_count,
			updated_at=EXCLUDED.updated_at
	), signal AS (
		INSERT INTO hg_youban_open_profile_signal
			(app_id,actor_id,profile_id,view_count,is_favorite,last_interaction_at,created_at,updated_at)
		SELECT ?,?,?,CASE WHEN ?='view' THEN 1 ELSE 0 END,CASE WHEN ?='favorite' THEN 1 ELSE 0 END,?,?,? FROM event
		ON CONFLICT (app_id,actor_id,profile_id) DO UPDATE SET
			view_count=hg_youban_open_profile_signal.view_count+EXCLUDED.view_count,
			is_favorite=CASE WHEN ?='view' THEN hg_youban_open_profile_signal.is_favorite ELSE EXCLUDED.is_favorite END,
			last_interaction_at=EXCLUDED.last_interaction_at,updated_at=EXCLUDED.updated_at
	)
	SELECT EXISTS (SELECT 1 FROM event) AS accepted`,
		appId, in.EventId, in.ActorId, in.ProfileId, in.Type, now, now,
		appId, in.ActorId, in.ProfileId, metricDate, now, in.Type,
		appId, in.ProfileId, metricDate, in.Type, in.Type, now, now,
		appId, in.ActorId, in.ProfileId, in.Type, in.Type, now, now, now, in.Type)
	if err != nil {
		return false, err
	}
	return row["accepted"].Bool(), nil
}

func insertInteractionEvent(ctx context.Context, tx gdb.TX, appId string, in *sysin.ProfileInteractionInp, now *gtime.Time) (bool, error) {
	sql := "INSERT IGNORE INTO " + interactionEventTable + " (app_id,event_id,actor_id,profile_id,event_type,occurred_at,created_at) VALUES (?,?,?,?,?,?,?)"
	if isOpenPgsql() {
		sql = "INSERT INTO " + interactionEventTable + " (app_id,event_id,actor_id,profile_id,event_type,occurred_at,created_at) VALUES (?,?,?,?,?,?,?) ON CONFLICT (app_id,event_id) DO NOTHING"
	}
	result, err := tx.Exec(sql, appId, in.EventId, in.ActorId, in.ProfileId, in.Type, now, now)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func insertDailyActor(ctx context.Context, tx gdb.TX, appId, actorId string, profileId int64, metricDate string, now *gtime.Time) (bool, error) {
	sql := "INSERT IGNORE INTO " + dailyActorTable + " (app_id,actor_id,profile_id,metric_date,created_at) VALUES (?,?,?,?,?)"
	if isOpenPgsql() {
		sql = "INSERT INTO " + dailyActorTable + " (app_id,actor_id,profile_id,metric_date,created_at) VALUES (?,?,?,?,?) ON CONFLICT (app_id,actor_id,profile_id,metric_date) DO NOTHING"
	}
	result, err := tx.Exec(sql, appId, actorId, profileId, metricDate, now)
	if err != nil {
		return false, err
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func upsertDailyMetric(ctx context.Context, tx gdb.TX, appId string, profileId int64, metricDate, eventType string, unique bool, now *gtime.Time) error {
	views, uniques, favorites := 0, 0, 0
	if eventType == "view" {
		views = 1
	}
	if unique {
		uniques = 1
	}
	if eventType == "favorite" {
		favorites = 1
	}
	if isOpenPgsql() {
		_, err := tx.Exec("INSERT INTO "+dailyMetricTable+" (app_id,profile_id,metric_date,view_count,unique_view_count,favorite_count,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT (app_id,profile_id,metric_date) DO UPDATE SET view_count="+dailyMetricTable+".view_count+EXCLUDED.view_count,unique_view_count="+dailyMetricTable+".unique_view_count+EXCLUDED.unique_view_count,favorite_count="+dailyMetricTable+".favorite_count+EXCLUDED.favorite_count,updated_at=EXCLUDED.updated_at", appId, profileId, metricDate, views, uniques, favorites, now, now)
		return err
	}
	_, err := tx.Exec("INSERT INTO "+dailyMetricTable+" (app_id,profile_id,metric_date,view_count,unique_view_count,favorite_count,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE view_count=view_count+VALUES(view_count),unique_view_count=unique_view_count+VALUES(unique_view_count),favorite_count=favorite_count+VALUES(favorite_count),updated_at=VALUES(updated_at)", appId, profileId, metricDate, views, uniques, favorites, now, now)
	return err
}

func upsertActorSignal(ctx context.Context, tx gdb.TX, appId string, in *sysin.ProfileInteractionInp, now *gtime.Time) error {
	favorite := 0
	if in.Type == "favorite" {
		favorite = 1
	}
	viewIncrement := 0
	if in.Type == "view" {
		viewIncrement = 1
	}
	if isOpenPgsql() {
		_, err := tx.Exec("INSERT INTO "+actorSignalTable+" (app_id,actor_id,profile_id,view_count,is_favorite,last_interaction_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?) ON CONFLICT (app_id,actor_id,profile_id) DO UPDATE SET view_count="+actorSignalTable+".view_count+EXCLUDED.view_count,is_favorite=CASE WHEN ?='view' THEN "+actorSignalTable+".is_favorite ELSE EXCLUDED.is_favorite END,last_interaction_at=EXCLUDED.last_interaction_at,updated_at=EXCLUDED.updated_at", appId, in.ActorId, in.ProfileId, viewIncrement, favorite, now, now, now, in.Type)
		return err
	}
	_, err := tx.Exec("INSERT INTO "+actorSignalTable+" (app_id,actor_id,profile_id,view_count,is_favorite,last_interaction_at,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?) ON DUPLICATE KEY UPDATE view_count=view_count+VALUES(view_count),is_favorite=IF(?='view',is_favorite,VALUES(is_favorite)),last_interaction_at=VALUES(last_interaction_at),updated_at=VALUES(updated_at)", appId, in.ActorId, in.ProfileId, viewIncrement, favorite, now, now, now, in.Type)
	return err
}

func isOpenPgsql() bool {
	return strings.EqualFold(g.DB().GetConfig().Type, "pgsql") || strings.EqualFold(g.DB().GetConfig().Type, "postgres")
}
