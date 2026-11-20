package sys

import (
	"context"
	"fmt"
	"html"
	"strconv"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	botin "hotgo/addons/youban_bot/model/input/sysin"
	botservice "hotgo/addons/youban_bot/service"
	"hotgo/addons/youban_publish/internal/dao"
	"hotgo/addons/youban_publish/internal/model/do"
	"hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	coredao "hotgo/internal/dao"
	"hotgo/internal/library/botregistry"
)

const (
	profileReviewTable          = "hg_youban_publish_profile_review"
	profileReviewRequest        = "profile_review_request"
	profileReviewPublish        = "profile_review_publish"
	profileReviewApprovedNotice = "profile_review_approved"
)

func reviewBotModel(ctx context.Context, tenantID, botID int64) *gdb.Model {
	return g.DB().Model(publishBotTable).Ctx(ctx).Where("tenant_id", tenantID).Where("id", botID).
		Where("bot_type", botregistry.RoleUploader).Where("status", 1).WhereNull("deleted_at")
}

func (s *sSysPublish) SubmitBotProfileReview(ctx context.Context, in *sysin.BotProfileReviewSubmit) error {
	capability, err := s.activeAccountCapability(ctx, in.TenantID, in.AccountID)
	if err != nil {
		return err
	}
	profileID, err := s.botResolveProfileId(ctx, in.TenantID, in.AccountID, in.ProfileNo, false)
	if err != nil {
		return err
	}
	return s.withProfileLifecycleLock(ctx, in.TenantID, profileID, func() error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			bot, err := reviewBotModel(ctx, capability.TenantId, in.BotID).One()
			if err != nil {
				return err
			}
			if bot.IsEmpty() || !bot["require_publish_review"].Bool() {
				return gerror.New("当前机器人未开启上架审核")
			}
			ownerID := bot["account_id"].Int64()
			if _, err = s.profileReviewAdmin(ctx, in.TenantID, ownerID); err != nil {
				return err
			}
			state, err := tx.Model(publishProfileStateTable).Where("tenant_id", in.TenantID).Where("account_id", in.AccountID).
				Where("profile_id", profileID).WhereNull("deleted_at").LockUpdate().One()
			if err != nil {
				return err
			}
			if state.IsEmpty() {
				return gerror.New("资料不存在或无权操作")
			}
			switch state["publish_task_status"].String() {
			case "pending", "publishing", "cycling", "waiting_media", "unpublishing", "canceling":
				return gerror.New("资料正在处理，请稍后提交审核")
			}
			cols := coredao.ContentProfile.Columns()
			profile := coredao.ContentProfile.Ctx(ctx).Where(cols.Id, profileID)
			row, err := profile.Clone().LockUpdate().Fields(cols.Title, cols.ProfileNo, cols.Status, cols.Visibility).One()
			if err != nil {
				return err
			}
			if row[cols.Status].Int() == sysin.ProfileStatusReviewPending {
				return gerror.New("资料正在审核中")
			}
			if row.IsEmpty() || row[cols.Status].Int() != 2 || row[cols.Visibility].String() == consts.ContentVisibilityPublic {
				return gerror.New("只有已下架的资料可以提交审核")
			}
			if _, err = profile.Data(g.Map{cols.Status: sysin.ProfileStatusReviewPending, cols.ReviewStatus: consts.ContentReviewPending, cols.Visibility: consts.ContentVisibilityPrivate}).Update(); err != nil {
				return err
			}
			reviewID, err := tx.Model(profileReviewTable).Data(g.Map{
				"tenant_id": in.TenantID, "bot_id": in.BotID, "owner_account_id": ownerID, "account_id": in.AccountID,
				"profile_id": profileID, "profile_no": row[cols.ProfileNo].String(), "title": row[cols.Title].String(), "applicant": in.Applicant, "chat_id": in.ChatID,
			}).InsertAndGetId()
			if err != nil {
				return err
			}
			if err = enqueueProfileReviewNotice(ctx, in.TenantID, ownerID, reviewID, profileReviewRequest); err != nil {
				return err
			}
			return s.syncProfileNoteIndex(ctx, profileID)
		})
	})
}

func (s *sSysPublish) profileReviewAdmin(ctx context.Context, tenantID, adminID int64) (*sysin.AccountCapabilityModel, error) {
	admin, err := s.activeAccountCapability(ctx, tenantID, adminID)
	if err != nil {
		return nil, err
	}
	if admin.AccountType != sysin.PublishAccountTypeAdmin {
		return nil, gerror.New("当前账号无权审批")
	}
	return admin, nil
}

func (s *sSysPublish) HandleBotProfileReview(ctx context.Context, id, adminID int64, action string) (*sysin.BotProfileReview, error) {
	var review *sysin.BotProfileReview
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if err := tx.Model(profileReviewTable).Where("id", id).Where("owner_account_id", adminID).LockUpdate().Scan(&review); err != nil {
			return err
		}
		if review == nil {
			return gerror.New("申请不存在或无权审批")
		}
		if _, err := s.profileReviewAdmin(ctx, review.TenantId, adminID); err != nil {
			return err
		}
		bot, err := reviewBotModel(ctx, review.TenantId, review.BotId).Where("account_id", adminID).Exist()
		if err != nil {
			return err
		}
		if !bot {
			return gerror.New("机器人已停用或归属已变更")
		}

		if action != "preview" && action != "approve" && action != "reject" {
			return gerror.New("审核操作无效")
		}
		owned, err := tx.Model(publishProfileStateTable).Where("tenant_id", review.TenantId).Where("account_id", review.AccountId).
			Where("profile_id", review.ProfileId).WhereNull("deleted_at").LockUpdate().One()
		if err != nil {
			return err
		}
		if owned.IsEmpty() {
			return gerror.New("资料归属已变更")
		}
		if review.Status != consts.ContentReviewPending {
			return gerror.New("该申请已处理")
		}
		if action == "preview" {
			return nil
		}
		status := consts.ContentReviewRejected
		if action == "approve" {
			status = consts.ContentReviewApproved
		}
		cols := coredao.ContentProfile.Columns()
		res, err := coredao.ContentProfile.Ctx(ctx).Where(cols.Id, review.ProfileId).Where(cols.Status, sysin.ProfileStatusReviewPending).
			Data(g.Map{cols.Status: 2, cols.ReviewStatus: status}).Update()
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return gerror.New("资料已删除或审核状态发生变化")
		}
		if _, err = tx.Model(profileReviewTable).Where("id", id).Data("status", status).Update(); err != nil {
			return err
		}
		review.Status = status
		if action == "approve" {
			if err = enqueueProfileReviewNotice(ctx, review.TenantId, adminID, id, profileReviewPublish); err != nil {
				return err
			}
			if err = enqueueProfileReviewNotice(ctx, review.TenantId, adminID, id, profileReviewApprovedNotice); err != nil {
				return err
			}
		}
		return s.syncProfileNoteIndex(ctx, review.ProfileId)
	})
	return review, err
}

func enqueueProfileReviewNotice(ctx context.Context, tenantID, ownerID, reviewID int64, kind string) error {
	_, err := dao.YoubanPublishAccountNotice.Ctx(ctx).Data(do.YoubanPublishAccountNotice{
		TenantId: tenantID, AccountId: ownerID, EventKey: fmt.Sprintf("%s:%d", kind, reviewID), Category: kind,
		TextContent: strconv.FormatInt(reviewID, 10), Status: "bot_pending",
	}).Insert()
	return err
}

func (s *sSysPublish) deliverProfileReviewNotice(ctx context.Context, notice *entity.YoubanPublishAccountNotice, id int64) error {
	var review *sysin.BotProfileReview
	err := g.DB().Model(profileReviewTable).Ctx(ctx).Where("id", id).Where("tenant_id", notice.TenantId).
		Where("owner_account_id", notice.AccountId).Scan(&review)
	if err != nil || review == nil {
		return err
	}
	if notice.Category == profileReviewApprovedNotice {
		if review.Status != consts.ContentReviewApproved {
			return nil
		}
		return botservice.SysBot().DeliverBotProfileApproval(ctx, review)
	}
	if notice.Category == profileReviewPublish {
		if review.Status != consts.ContentReviewApproved {
			return nil
		}
		latest, err := g.DB().Model(profileReviewTable).Ctx(ctx).Where("profile_id", review.ProfileId).
			Where("tenant_id", review.TenantId).OrderDesc("id").Fields("id").Value()
		if err != nil || latest.Int64() != id {
			return err
		}
		if _, err := s.activeAccountCapability(ctx, review.TenantId, review.AccountId); err != nil {
			return err
		}
		operationNo := fmt.Sprintf("profile:review:%d", id)
		return s.submitProfilePublish(ctx, review.ProfileId, review.TenantId, review.AccountId, review.OwnerAccountId, operationNo, nil, false)
	}
	if review.Status != consts.ContentReviewPending {
		return nil
	}
	if _, err = s.profileReviewAdmin(ctx, review.TenantId, review.OwnerAccountId); err != nil {
		return err
	}
	bot, err := reviewBotModel(ctx, review.TenantId, review.BotId).Where("account_id", review.OwnerAccountId).Exist()
	if err != nil || !bot {
		return err
	}
	return botservice.SysBot().NotifyAccount(ctx, &botin.NotifyAccountInp{
		App: "api", AccountId: review.OwnerAccountId, BotStrategy: "official", IgnoreFeatureSwitch: true, RequireDelivery: true,
		Text: fmt.Sprintf("收到资料上架审核\n标题：%s\n编号：%s\n上架用户：%s\n上架时间：%s",
			html.EscapeString(review.Title), html.EscapeString(review.ProfileNo), html.EscapeString(review.Applicant), review.CreatedAt.String()),
		Buttons: [][]botin.NotifyButton{
			{{Label: "预览", Data: fmt.Sprintf("pur:preview:%d", id)}},
			{{Label: "同意", Data: fmt.Sprintf("pur:approve:%d", id)}, {Label: "拒绝", Data: fmt.Sprintf("pur:reject:%d", id)}},
		},
	})
}

func checkProfileReviewPublishable(ctx context.Context, profileID int64) error {
	cols := coredao.ContentProfile.Columns()
	pending, err := coredao.ContentProfile.Ctx(ctx).Where(cols.Id, profileID).Where(cols.Status, sysin.ProfileStatusReviewPending).Exist()
	if err != nil {
		return err
	}
	if pending {
		return errPublishProfileUnavailable
	}
	return nil
}
