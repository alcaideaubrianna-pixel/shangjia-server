package sys

import (
	"context"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	iservice "hotgo/internal/service"
)

const collectProfileSourceType = "youban_collect"

func (s *sSysPublish) upsertCollectProfile(ctx context.Context, event gdb.Record, content *collectContentResult, rule gdb.Record, text string) (int64, error) {
	text = strings.TrimSpace(text)
	title := collectTitle(text)
	if title == "" {
		return 0, gerror.New("采集资料标题为空")
	}
	tenantId := event["tenant_id"].Int64()
	accountId := event["account_id"].Int64()
	if tenantId <= 0 || accountId <= 0 {
		return 0, gerror.New("采集资料归属不完整")
	}
	sourceKey := collectPublishClientRequestId(event, rule)
	channelJSON := rule["target_channel_id_json"].String()
	now := gtime.Now()
	var profileId int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		columns := dao.ContentProfile.Columns()
		existing, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).
			Fields(columns.Id).
			Where(columns.SourceKey, sourceKey).
			WhereNull(columns.DeletedAt).
			One()
		if err != nil {
			return gerror.Wrap(err, "读取采集资料失败")
		}
		data := g.Map{
			columns.SourceType:      collectProfileSourceType,
			columns.SourceKey:       sourceKey,
			columns.SourceTextHash:  collectHash(text),
			columns.Title:           title,
			columns.Summary:         profileSummary(text),
			columns.PlainText:       text,
			columns.Visibility:      consts.ContentVisibilityPrivate,
			columns.ReviewStatus:    consts.ContentReviewApproved,
			columns.ImportStatus:    "collect",
			columns.SourceUpdateBy:  strconv.FormatInt(accountId, 10),
			columns.SourceUpdatedAt: now,
			columns.Status:          2,
			columns.PublishedAt:     nil,
			columns.UpdatedAt:       now,
			columns.DeletedAt:       nil,
		}
		if existing.IsEmpty() {
			uuid := newPublishProfileUUID()
			data[columns.SourceNoteUuid] = uuid
			data[columns.SourceCreateBy] = strconv.FormatInt(accountId, 10)
			data[columns.SourceCreatedAt] = now
			data[columns.CreatedAt] = now
			for i := 0; i < 1000; i++ {
				profileNo, numberErr := s.nextAccountProfileNo(ctx, tx, tenantId, accountId)
				if numberErr != nil {
					return numberErr
				}
				data[columns.ProfileNo] = profileNo
				profileId, err = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
				if err == nil {
					break
				}
				if !isProfileNoUniqueConstraintError(err) {
					return gerror.Wrap(err, "创建采集资料失败")
				}
			}
			if profileId <= 0 {
				return gerror.New("创建采集资料失败")
			}
		} else {
			profileId = existing[columns.Id].Int64()
			if _, err = tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Where(columns.Id, profileId).Data(data).Update(); err != nil {
				return gerror.Wrap(err, "更新采集资料失败")
			}
		}
		return s.upsertProfileStateTx(ctx, tx, profileId, tenantId, accountId, channelJSON, "", 0, nil)
	})
	if err != nil {
		return 0, err
	}
	if err = s.rebuildCollectProfileMedia(ctx, event, content, profileId); err != nil {
		return 0, err
	}
	if err = s.syncProfileNoteIndex(ctx, profileId); err != nil {
		return 0, err
	}
	iservice.SysContent().ClearHomeProfileCardsCache(ctx)
	return profileId, nil
}
