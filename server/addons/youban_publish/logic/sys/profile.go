package sys

import (
	"context"
	"fmt"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
)

const publishProfileSourceType = "youban_publish"

func (s *sSysPublish) publishTaskToProfile(ctx context.Context, task gdb.Record) (profileId int64, err error) {
	if task.IsEmpty() {
		return 0, gerror.New("上架任务不存在")
	}
	if task["profile_id"].Int64() > 0 {
		return task["profile_id"].Int64(), nil
	}
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		locked, lockErr := tx.Model(publishTaskTable).Ctx(ctx).
			Where("id", task["id"].Int64()).
			WhereNull("deleted_at").
			One()
		if lockErr != nil {
			return gerror.Wrap(lockErr, "读取上架任务失败")
		}
		if locked.IsEmpty() {
			return gerror.New("上架任务不存在")
		}
		if locked["profile_id"].Int64() > 0 {
			profileId = locked["profile_id"].Int64()
			return nil
		}
		id, createErr := s.createContentProfile(ctx, tx, locked)
		if createErr != nil {
			return createErr
		}
		now := gtime.Now()
		if _, updateErr := tx.Model(publishTaskTable).Ctx(ctx).
			Where("id", locked["id"].Int64()).
			Data(g.Map{
				"profile_id": id,
				"updated_at": now,
			}).
			Update(); updateErr != nil {
			return gerror.Wrap(updateErr, "回写资料ID失败")
		}
		profileId = id
		if syncErr := s.syncTaskMediaToProfile(ctx, tx, locked["id"].Int64(), id); syncErr != nil {
			return syncErr
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return profileId, nil
}

func (s *sSysPublish) createContentProfile(ctx context.Context, tx gdb.TX, task gdb.Record) (int64, error) {
	columns := dao.ContentProfile.Columns()
	now := gtime.Now()
	sourceKey := publishProfileSourceKey(task)
	profileNo := publishProfileNo(task)
	data := g.Map{
		columns.ProfileNo:       profileNo,
		columns.SourceType:      publishProfileSourceType,
		columns.SourceKey:       sourceKey,
		columns.Title:           strings.TrimSpace(task["title"].String()),
		columns.Summary:         profileSummary(task["plain_text"].String()),
		columns.PlainText:       strings.TrimSpace(task["plain_text"].String()),
		columns.Province:        strings.TrimSpace(task["province"].String()),
		columns.City:            strings.TrimSpace(task["city"].String()),
		columns.ImageCount:      task["media_count"].Int(),
		columns.VideoCount:      0,
		columns.Visibility:      consts.ContentVisibilityPrivate,
		columns.ReviewStatus:    consts.ContentReviewPending,
		columns.ImportStatus:    "pending",
		columns.AdminRemark:     fmt.Sprintf("youban_publish task:%d tenant:%d account:%d", task["id"].Int64(), task["tenant_id"].Int64(), task["account_id"].Int64()),
		columns.SourceCreateBy:  fmt.Sprintf("%d", task["account_id"].Int64()),
		columns.SourceUpdateBy:  fmt.Sprintf("%d", task["account_id"].Int64()),
		columns.SourceCreatedAt: now,
		columns.SourceUpdatedAt: now,
		columns.Status:          2,
		columns.CreatedAt:       now,
		columns.UpdatedAt:       now,
	}
	id, err := tx.Model(dao.ContentProfile.Table()).Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return 0, gerror.Wrap(err, "创建主资料失败")
	}
	return id, nil
}

func publishProfileSourceKey(task gdb.Record) string {
	clientRequestId := strings.TrimSpace(task["client_request_id"].String())
	if clientRequestId != "" {
		return fmt.Sprintf("youban_publish:%d:%s", task["tenant_id"].Int64(), clientRequestId)
	}
	return fmt.Sprintf("youban_publish:task:%d", task["id"].Int64())
}

func publishProfileNo(task gdb.Record) string {
	return fmt.Sprintf("YBP%d", task["id"].Int64())
}

func profileSummary(text string) string {
	text = strings.TrimSpace(text)
	if len([]rune(text)) <= 80 {
		return text
	}
	return string([]rune(text)[:80])
}
