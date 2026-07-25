package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/dao"
)

type noteIndexSource struct {
	TenantId        int64       `orm:"tenant_id"`
	AccountId       int64       `orm:"account_id"`
	ProfileId       int64       `orm:"profile_id"`
	TaskId          int64       `orm:"task_id"`
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
		InnerJoin(publishTaskTable+" t", "t.profile_id=p.id AND t.deleted_at IS NULL").
		Where("p.id", profileId).
		WhereNull("p.deleted_at").
		Fields("t.tenant_id,t.account_id,p.id AS profile_id,t.id AS task_id,p.source_note_uuid AS uuid,p.profile_no,p.title,p.summary,p.plain_text," + tag + " AS tag,p.province,p.city,p.status,p.visibility,p.review_status,t.status AS task_status,p.published_at,COALESCE(p.updated_at,t.updated_at) AS source_updated_at,p.created_at,p.updated_at,p.deleted_at").
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
		columns.TaskId:          row.TaskId,
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
	return nil
}

func noteIndexModel(ctx context.Context) *gdb.Model {
	return pdao.YoubanPublishNoteIndex.Ctx(ctx).As("i").WhereNull("i.deleted_at")
}
