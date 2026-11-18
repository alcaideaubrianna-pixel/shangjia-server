package sys

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hotgo/addons/youban_publish/model/input/sysin"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const profileCycleOverrideRebuildEvent = "profile.cycle_override.rebuild"

type profileCycleOverrideIntent struct {
	TenantID  int64 `json:"tenantId"`
	AccountID int64 `json:"accountId"`
	ProfileID int64 `json:"profileId"`
	Version   int64 `json:"version"`
}

// AdminProfileCycleOverride commits configuration and durable rebuild intent together.
func (s *sSysPublish) AdminProfileCycleOverride(ctx context.Context, in *sysin.ProfileCycleOverrideInp) error {
	if in == nil {
		return gerror.New("资料循环配置不能为空")
	}
	if err := in.Filter(ctx); err != nil {
		return err
	}
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	profile, err := s.profileBaseBySelector(ctx, &in.ProfileViewInp, account.TenantId, account.Id)
	if err != nil {
		return err
	}
	days := in.CycleOverrideDays
	if in.CycleOverrideEnabled {
		vip, vipErr := s.tenantVipStatus(ctx, profile.TenantId)
		if vipErr != nil {
			return vipErr
		}
		if !tenantVipStatusActive(vip) {
			days = maxConfigInt(ctx, "youbanPublish.cycle.freeIntervalDays", 15)
			if days <= 0 {
				days = 15
			}
		}
	}
	err = s.withProfileLifecycleLock(ctx, profile.TenantId, profile.Id, func() error {
		return g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
			state, lockErr := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("tenant_id", account.TenantId).Where("account_id", account.Id).
				Where("profile_id", profile.Id).WhereNull("deleted_at").
				Fields("cycle_override_enabled,cycle_override_days,cycle_override_version").
				LockUpdate().One()
			if lockErr != nil {
				return gerror.Wrap(lockErr, "锁定资料循环配置失败")
			}
			if state.IsEmpty() {
				return gerror.New("资料不存在或无权操作")
			}
			// A zero value while disabling preserves the previous custom interval.
			savedDays := days
			if !in.CycleOverrideEnabled && savedDays == 0 {
				savedDays = state["cycle_override_days"].Int()
			}
			// Synchronize every mapping in one statement, including on repeated
			// saves. Only actual true-to-false projections rejoin the batch tail;
			// status and existing inherited mappings retain their current values.
			projection := g.Map{"cycle_override_enabled": in.CycleOverrideEnabled}
			if !in.CycleOverrideEnabled {
				projection["batch_order"] = gdb.Raw("nextval('hg_ybp_batch_order_seq')")
			}
			_, projectionErr := tx.Model(publishChannelProfileTable).Safe().Ctx(ctx).
				Where("tenant_id", account.TenantId).Where("account_id", account.Id).
				Where("profile_id", profile.Id).
				Where("cycle_override_enabled", !in.CycleOverrideEnabled).
				Data(projection).Update()
			if projectionErr != nil {
				return gerror.Wrap(projectionErr, "同步资料循环映射失败")
			}
			version := state["cycle_override_version"].Int64()
			if state["cycle_override_enabled"].Bool() == in.CycleOverrideEnabled && state["cycle_override_days"].Int() == savedDays {
				// Repeated saves only wake the existing pending intent. Never
				// reactivate completed work or manufacture another version.
				_, wakeErr := tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).
					Where("event_type", profileCycleOverrideRebuildEvent).
					Where("dedupe_key", fmt.Sprintf("profile-cycle:%d:%d:%d:%d", profile.TenantId, profile.AccountId, profile.Id, version)).
					Where("status", telegramOutboxPending).
					Data(g.Map{"available_at": gtime.Now(), "last_error": ""}).Update()
				if wakeErr != nil {
					return gerror.Wrap(wakeErr, "唤醒资料循环重算任务失败")
				}
				return nil
			}
			if version == int64(1<<63-1) {
				return gerror.New("资料循环配置版本已达到上限")
			}
			version++
			updateResult, updateErr := tx.Model(publishProfileStateTable).Safe().Ctx(ctx).
				Where("tenant_id", account.TenantId).Where("account_id", account.Id).
				Where("profile_id", profile.Id).WhereNull("deleted_at").
				Where("cycle_override_version", version-1).
				Data(g.Map{"cycle_override_enabled": in.CycleOverrideEnabled,
					"cycle_override_days": savedDays, "cycle_override_version": version}).Update()
			if updateErr != nil {
				return gerror.Wrap(updateErr, "保存资料循环配置失败")
			}
			affected, affectedErr := updateResult.RowsAffected()
			if affectedErr != nil {
				return gerror.Wrap(affectedErr, "读取资料循环配置保存结果失败")
			}
			if affected != 1 {
				return gerror.New("资料循环配置已变更，请刷新后重试")
			}
			intent := profileCycleOverrideIntent{TenantID: profile.TenantId, AccountID: profile.AccountId, ProfileID: profile.Id, Version: version}
			body, encodeErr := json.Marshal(intent)
			if encodeErr != nil {
				return gerror.Wrap(encodeErr, "编码资料循环重算任务失败")
			}
			insertResult, insertErr := tx.Model(publishTgOutboxTable).Safe().Ctx(ctx).Data(g.Map{
				"job_id": 0, "event_type": profileCycleOverrideRebuildEvent,
				"dedupe_key": fmt.Sprintf("profile-cycle:%d:%d:%d:%d", intent.TenantID, intent.AccountID, intent.ProfileID, version),
				"payload":    string(body), "status": telegramOutboxPending, "available_at": gtime.Now(),
			}).Insert()
			if insertErr != nil {
				return gerror.Wrap(insertErr, "保存资料循环重算任务失败")
			}
			affected, affectedErr = insertResult.RowsAffected()
			if affectedErr != nil {
				return gerror.Wrap(affectedErr, "读取资料循环重算任务保存结果失败")
			}
			if affected != 1 {
				return gerror.New("资料循环重算任务未保存成功，请重试")
			}
			return nil
		})
	})
	return err
}

// dispatchPendingProfileCycleOverrides is the integration point for the existing Outbox recovery loop.
// Repeated delivery is safe: rebuild reads current configuration, never an old payload's values.
func (s *sSysPublish) dispatchPendingProfileCycleOverrides(ctx context.Context, limit int) error {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
		Where("event_type", profileCycleOverrideRebuildEvent).Where("status", telegramOutboxPending).
		WhereLTE("available_at", gtime.Now()).Fields("id,payload").OrderAsc("available_at,id").Limit(limit).All()
	if err != nil {
		return gerror.Wrap(err, "读取资料循环重算任务失败")
	}
	var firstErr error
	for _, row := range rows {
		var intent profileCycleOverrideIntent
		waitingForProjection := false
		deliveryErr := json.Unmarshal([]byte(row["payload"].String()), &intent)
		if deliveryErr == nil && (intent.TenantID <= 0 || intent.AccountID <= 0 || intent.ProfileID <= 0 || intent.Version <= 0) {
			deliveryErr = gerror.New("资料循环重算任务归属不合法")
		}
		if deliveryErr == nil {
			state, readErr := g.DB().Model(publishProfileStateTable).Safe().Ctx(ctx).Master().
				Where("tenant_id", intent.TenantID).Where("account_id", intent.AccountID).
				Where("profile_id", intent.ProfileID).WhereNull("deleted_at").
				Fields("profile_id,cycle_override_version").One()
			deliveryErr = readErr
			if deliveryErr == nil && !state.IsEmpty() {
				pending, planErr := g.DB().Model(publishChannelProfileTable).Safe().Ctx(ctx).Master().
					Where("tenant_id", intent.TenantID).Where("account_id", intent.AccountID).
					Where("profile_id", intent.ProfileID).Where("status", "active").
					WhereGT("last_job_id", 0).
					WhereLT("applied_profile_cycle_version", state["cycle_override_version"].Int64()).
					Fields("id").Limit(1).One()
				deliveryErr = planErr
				if deliveryErr == nil && !pending.IsEmpty() {
					waitingForProjection = true
					deliveryErr = s.rebuildProfileCycleOverridePlans(ctx, intent.TenantID, intent.AccountID, intent.ProfileID)
				}
			}
		}
		update := g.Map{"status": telegramOutboxDelivered, "delivered_at": gtime.Now(), "last_error": ""}
		if waitingForProjection {
			// Redis acceptance is not completion. Keep the durable intent until
			// the existing worker has committed the mapping version to SQL.
			update = g.Map{"last_error": "", "available_at": gtime.Now().Add(30 * time.Second)}
		}
		if deliveryErr != nil {
			if firstErr == nil {
				firstErr = deliveryErr
			}
			update = g.Map{"last_error": "资料循环重算失败，等待重试", "available_at": gtime.Now().Add(30 * time.Second)}
		}
		_, updateErr := g.DB().Model(publishTgOutboxTable).Safe().Ctx(ctx).
			Where("id", row["id"].Int64()).Where("event_type", profileCycleOverrideRebuildEvent).
			Where("status", telegramOutboxPending).Data(update).Update()
		if updateErr != nil && firstErr == nil {
			firstErr = gerror.Wrap(updateErr, "更新资料循环重算任务失败")
		}
	}
	return firstErr
}
