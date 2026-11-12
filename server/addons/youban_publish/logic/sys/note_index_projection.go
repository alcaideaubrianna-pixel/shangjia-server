package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/dao"
)

const noteIndexCollectedColumn = "is_collected"

type noteIndexSource struct {
	IsCollected     bool        `orm:"is_collected"`
	TenantId        int64       `orm:"tenant_id"`
	AccountId       int64       `orm:"account_id"`
	ProfileId       int64       `orm:"profile_id"`
	Uuid            string      `orm:"uuid"`
	ProfileNo       string      `orm:"profile_no"`
	Title           string      `orm:"title"`
	Summary         string      `orm:"summary"`
	PlainText       string      `orm:"plain_text"`
	Tag             string      `orm:"tag"`
	Province        string      `orm:"province"`
	City            string      `orm:"city"`
	Status          int         `orm:"status"`
	Visibility      string      `orm:"visibility"`
	ReviewStatus    string      `orm:"review_status"`
	TaskStatus      string      `orm:"task_status"`
	PublishedAt     *gtime.Time `orm:"published_at"`
	SourceUpdatedAt *gtime.Time `orm:"source_updated_at"`
	CreatedAt       *gtime.Time `orm:"created_at"`
	UpdatedAt       *gtime.Time `orm:"updated_at"`
	DeletedAt       *gtime.Time `orm:"deleted_at"`
}

func (s *sSysPublish) RefreshNoteIndex(ctx context.Context, profileId int64) error {
	return s.syncProfileNoteIndex(ctx, profileId)
}

func (s *sSysPublish) refreshProfileNoteIndexes(ctx context.Context, profileIds []int64) error {
	profileIds = uniqueIds(profileIds)
	if len(profileIds) == 0 {
		return nil
	}
	for _, profileId := range profileIds {
		if err := s.syncProfileNoteIndex(ctx, profileId); err != nil {
			return err
		}
	}
	return nil
}

// syncProfileNoteIndex refreshes every active tenant/account projection for a profile.
// It is called after the source transaction commits so the read model never exposes
// a task that the source transaction rolled back.
func (s *sSysPublish) syncProfileNoteIndex(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	columns := pdao.YoubanPublishNoteIndex.Columns()
	if _, err := pdao.YoubanPublishNoteIndex.Ctx(ctx).
		Where("profile_id", profileId).
		Data(g.Map{columns.DeletedAt: gtime.Now(), columns.UpdatedAt: gtime.Now()}).
		Update(); err != nil {
		return gerror.Wrap(err, "清理资料索引失败")
	}
	if _, err := g.DB().Model("hg_youban_publish_note_search").Ctx(ctx).
		Where("profile_id", profileId).Data(g.Map{"deleted_at": gtime.Now(), "updated_at": gtime.Now()}).Update(); err != nil {
		return gerror.Wrap(err, "清理资料搜索索引失败")
	}

	rows, err := s.noteIndexSources(ctx, profileId)
	if err != nil {
		return err
	}
	for i := range rows {
		if err = upsertNoteIndex(ctx, &rows[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) noteIndexSources(ctx context.Context, profileId int64) ([]noteIndexSource, error) {
	tag := profileTagFieldExpr()
	var rows []noteIndexSource
	err := g.DB().Model(dao.ContentProfile.Table()+" p").Safe().Ctx(ctx).
		InnerJoin(publishProfileStateTable+" ps", "ps.profile_id=p.id AND ps.deleted_at IS NULL").
		Where("p.id", profileId).
		WhereNull("p.deleted_at").
		Fields("ps.tenant_id,ps.account_id,p.id AS profile_id,COALESCE(p.source_type='" + collectProfileSourceType + "',false) AS " + noteIndexCollectedColumn + ",p.source_note_uuid AS uuid,p.profile_no,p.title,p.summary,p.plain_text," + tag + " AS tag,p.province,p.city,p.status,p.visibility,p.review_status,COALESCE(ps.publish_task_status,'') AS task_status,p.published_at,p.updated_at AS source_updated_at,p.created_at,p.updated_at,p.deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取资料索引源数据失败")
	}
	return rows, nil
}

func upsertNoteIndex(ctx context.Context, row *noteIndexSource) error {
	if row == nil || row.TenantId <= 0 || row.AccountId <= 0 || row.ProfileId <= 0 {
		return nil
	}
	columns := pdao.YoubanPublishNoteIndex.Columns()
	data := g.Map{
		columns.TenantId:        row.TenantId,
		columns.AccountId:       row.AccountId,
		columns.ProfileId:       row.ProfileId,
		columns.Uuid:            row.Uuid,
		columns.ProfileNo:       row.ProfileNo,
		columns.Title:           row.Title,
		columns.Summary:         row.Summary,
		columns.PlainText:       row.PlainText,
		columns.Tag:             row.Tag,
		columns.Province:        row.Province,
		columns.City:            row.City,
		columns.Status:          row.Status,
		columns.Visibility:      row.Visibility,
		columns.ReviewStatus:    row.ReviewStatus,
		columns.TaskStatus:      row.TaskStatus,
		columns.PublishedAt:     row.PublishedAt,
		columns.SourceUpdatedAt: row.SourceUpdatedAt,
		columns.CreatedAt:       row.CreatedAt,
		columns.UpdatedAt:       row.UpdatedAt,
		columns.DeletedAt:       row.DeletedAt,
	}
	data[noteIndexCollectedColumn] = row.IsCollected
	mod := pdao.YoubanPublishNoteIndex.Ctx(ctx).Data(data)
	// The projection table may have been created by an older installation without
	// a primary key. Declare the business unique key explicitly so Save does not
	// depend on ORM primary-key metadata.
	mod = mod.
		OnConflict("tenant_id,account_id,profile_id").
		OnDuplicateEx("id,tenant_id,account_id,profile_id,created_at")
	_, err := mod.Save()
	if err != nil {
		return gerror.Wrap(err, "写入资料索引失败")
	}
	if err := upsertNoteIndexSearch(ctx, row); err != nil {
		return err
	}
	return nil
}

func upsertNoteIndexSearch(ctx context.Context, row *noteIndexSource) error {
	if row == nil || row.TenantId <= 0 || row.AccountId <= 0 || row.ProfileId <= 0 {
		return nil
	}
	searchText := strings.Join([]string{row.ProfileNo, row.Title, row.Summary, row.PlainText}, " ")
	_, err := g.DB().Model("hg_youban_publish_note_search").Ctx(ctx).Data(g.Map{
		"tenant_id": row.TenantId, "account_id": row.AccountId,
		"profile_id": row.ProfileId, "is_collected": row.IsCollected, "status": row.Status,
		"search_text": searchText, "deleted_at": row.DeletedAt, "updated_at": gtime.Now(),
	}).OnConflict("tenant_id,account_id,profile_id").OnDuplicateEx("id,tenant_id,account_id,profile_id").Save()
	if err != nil {
		return gerror.Wrap(err, "写入资料搜索索引失败")
	}
	return nil
}

func (s *sSysPublish) deleteProfileNoteIndex(ctx context.Context, profileIds []int64) error {
	if len(profileIds) == 0 {
		return nil
	}
	columns := pdao.YoubanPublishNoteIndex.Columns()
	_, err := pdao.YoubanPublishNoteIndex.Ctx(ctx).
		WhereIn(columns.ProfileId, profileIds).
		Data(g.Map{columns.DeletedAt: gtime.Now(), columns.UpdatedAt: gtime.Now()}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "删除资料索引失败")
	}
	if _, err = g.DB().Model("hg_youban_publish_note_search").Ctx(ctx).
		WhereIn("profile_id", profileIds).
		Data(g.Map{"deleted_at": gtime.Now(), "updated_at": gtime.Now()}).Update(); err != nil {
		return gerror.Wrap(err, "删除资料搜索索引失败")
	}
	return nil
}

func noteIndexModel(ctx context.Context) *gdb.Model {
	return pdao.YoubanPublishNoteIndex.Ctx(ctx).As("i").
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=i.profile_id AND p.deleted_at IS NULL").
		InnerJoin(publishProfileStateTable+" ps", "ps.tenant_id=i.tenant_id AND ps.account_id=i.account_id AND ps.profile_id=i.profile_id AND ps.deleted_at IS NULL").
		WhereNull("i.deleted_at")
}
