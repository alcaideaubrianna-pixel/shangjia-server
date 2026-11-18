package sys

import (
	"context"
	"hotgo/internal/dao"
	"strconv"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

type batchRotationCandidate struct {
	ProfileID int64 `orm:"profile_id"`
	AccountID int64 `orm:"account_id"`
	Order     int64 `orm:"batch_order"`
}

// batchRotationEligibilitySQL shares the full-push account/channel authorization
// rules, but deliberately has no job history, task status or media condition.
// cp must denote a channel-profile row.
func batchRotationEligibilitySQL() string {
	return `EXISTS (SELECT 1 FROM ` + dao.ContentProfile.Table() + ` p
		WHERE p.id=cp.profile_id AND p.status=1 AND p.visibility='public' AND p.deleted_at IS NULL)
		AND EXISTS (SELECT 1 FROM ` + publishProfileStateTable + ` ps
		WHERE ps.profile_id=cp.profile_id AND ps.tenant_id=cp.tenant_id
		  AND ps.account_id=cp.account_id AND ps.deleted_at IS NULL
		  AND ps.cycle_override_enabled=false
		  AND (cp.account_id=0 OR EXISTS (SELECT 1 FROM ` + publishAccountTable + ` a
		    WHERE a.id=ps.account_id AND a.tenant_id=cp.tenant_id AND a.status=1 AND a.deleted_at IS NULL)))
		AND (
		  EXISTS (SELECT 1 FROM ` + publishProfileChannelTable + ` pc
		    WHERE pc.tenant_id=cp.tenant_id AND pc.account_id=cp.account_id
		      AND pc.profile_id=cp.profile_id AND pc.channel_id=cp.channel_id
		      AND pc.is_manual=1 AND pc.deleted_at IS NULL)
		  OR (
		    NOT EXISTS (SELECT 1 FROM ` + publishProfileChannelTable + ` pc
		      WHERE pc.tenant_id=cp.tenant_id AND pc.account_id=cp.account_id
		        AND pc.profile_id=cp.profile_id AND pc.is_manual=1 AND pc.deleted_at IS NULL)
		    AND EXISTS (SELECT 1 FROM ` + publishChannelTable + ` dc
		      WHERE dc.id=cp.channel_id AND dc.tenant_id=cp.tenant_id
		        AND dc.publish_direction='up' AND dc.status=1
		        AND dc.is_default_selected=1 AND dc.deleted_at IS NULL)))`
}

// batchRotationCandidatesTx reads bounded, materialized index pages before
// checking profiles by primary key. Invalid rows advance the keyset as well.
// Callers serialize scheduling per channel; this function does not reserve jobs.
func batchRotationCandidatesTx(ctx context.Context, tx gdb.TX, tenantID, channelID int64, limit int) ([]batchRotationCandidate, error) {
	items := make([]batchRotationCandidate, 0)
	if tenantID <= 0 || channelID <= 0 || limit <= 0 {
		return items, nil
	}
	if limit > 5000 {
		return nil, gerror.New("批次轮播每日数量不能超过5000份")
	}
	if err := batchRotationIndexReadyTx(ctx, tx); err != nil {
		return nil, err
	}
	upper, err := tx.Ctx(ctx).GetAll(`SELECT batch_order,profile_id FROM `+publishChannelProfileTable+`
		WHERE tenant_id=? AND channel_id=? AND status='active' AND batch_order>0 AND cycle_override_enabled=false
		ORDER BY batch_order DESC,profile_id DESC LIMIT 1`, tenantID, channelID)
	if err != nil {
		return nil, gerror.Wrap(err, "读取频道轮播扫描边界失败")
	}
	if len(upper) == 0 {
		return items, nil
	}
	upperOrder, upperProfile := upper[0]["batch_order"].Int64(), upper[0]["profile_id"].Int64()
	var order, profileID int64
	const scanBudget = 10000
	scanned := 0
	for len(items) < limit && scanned < scanBudget {
		pageSize := min(500, limit-len(items), scanBudget-scanned)
		// Freeze the original tail: rotating rejected rows must not make this
		// bounded scan visit them repeatedly in the same transaction.
		rows, err := tx.Ctx(ctx).GetAll(batchRotationCandidatePageSQL(),
			tenantID, channelID, order, profileID, upperOrder, upperProfile, pageSize)
		if err != nil {
			return nil, gerror.Wrap(err, "读取批次轮播候选资料失败")
		}
		if len(rows) == 0 {
			return items, nil
		}
		rejected := make([]batchRotationCandidate, 0, len(rows))
		for _, row := range rows {
			order, profileID = row["batch_order"].Int64(), row["profile_id"].Int64()
			candidate := batchRotationCandidate{ProfileID: profileID, AccountID: row["account_id"].Int64(), Order: order}
			if row["eligible"].Bool() {
				items = append(items, candidate)
			} else {
				rejected = append(rejected, candidate)
			}
		}
		scanned += len(rows)
		if err := rotateBatchRotationRejectedTx(ctx, tx, tenantID, channelID, rejected); err != nil {
			return nil, err
		}
		if len(rows) < pageSize || (order == upperOrder && profileID == upperProfile) {
			return items, nil
		}
	}
	if len(items) < limit && scanned >= scanBudget {
		return items, errBatchRotationScanBudget
	}
	return items, nil
}

// admitBatchRotationProfileTx must be called in the job-creation transaction after
// authorization. It admits first-time online profiles without requiring a sent
// job. Only explicit newly created jobs may reactivate inactive mappings.
// Success callbacks never call this helper. Active queue positions are preserved.
func (s *sSysPublish) admitBatchRotationProfileTx(ctx context.Context, tx gdb.TX, job telegramJobRecord) error {
	if strings.HasPrefix(strings.ToLower(job.OperationNo), "down:") {
		return nil
	}
	tenantID, accountID, channelID, profileID := job.TenantId, job.AccountId, job.ChannelId, job.ProfileId
	if tenantID <= 0 || accountID < 0 || channelID <= 0 || profileID <= 0 || job.Id <= 0 {
		return gerror.New("批次轮播入队参数不合法")
	}
	_, err := tx.Ctx(ctx).Exec(`INSERT INTO `+publishChannelProfileTable+` AS cp
		(tenant_id,account_id,channel_id,profile_id,status,batch_order,cycle_override_enabled)
		SELECT ?,?,?,?,'active',nextval('hg_ybp_batch_order_seq'),
		  COALESCE((SELECT ps.cycle_override_enabled FROM `+publishProfileStateTable+` ps
		    WHERE ps.tenant_id=? AND ps.account_id=? AND ps.profile_id=? AND ps.deleted_at IS NULL LIMIT 1),false)
		WHERE EXISTS (SELECT 1 FROM `+dao.ContentProfile.Table()+` p
		  WHERE p.id=? AND p.status=1 AND p.visibility='public' AND p.deleted_at IS NULL)
		ON CONFLICT (channel_id,profile_id) DO UPDATE
		SET status='active',batch_order=nextval('hg_ybp_batch_order_seq')
		WHERE cp.tenant_id=EXCLUDED.tenant_id AND cp.account_id=EXCLUDED.account_id
		  AND ((cp.status='active' AND cp.batch_order=0)
		    OR (cp.status='inactive' AND cp.last_job_id<? AND cp.batch_last_sent_job_id<?))`,
		tenantID, accountID, channelID, profileID, tenantID, accountID, profileID, profileID, job.Id, job.Id)
	if err != nil {
		return gerror.Wrap(err, "批次轮播资料入队失败")
	}
	return nil
}

// reserveBatchRotationCandidatesTx moves claimed attempts to the tail without
// changing publication status or success watermarks. Persist the snapshot with
// original candidate.Order in this SAME transaction. Any error requires rollback
// of the entire caller transaction, including snapshot inserts.
func reserveBatchRotationCandidatesTx(ctx context.Context, tx gdb.TX, tenantID, channelID int64, candidates []batchRotationCandidate) error {
	if len(candidates) == 0 {
		return nil
	}
	if tenantID <= 0 || channelID <= 0 || len(candidates) > 5000 {
		return gerror.New("批次轮播领取参数不合法")
	}
	values := make([]string, 0, len(candidates))
	args := make([]interface{}, 0, len(candidates)*3+2)
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.ProfileID <= 0 || candidate.AccountID < 0 || candidate.Order <= 0 {
			return gerror.New("批次轮播候选身份或顺序不合法")
		}
		if _, exists := seen[candidate.ProfileID]; exists {
			return gerror.New("批次轮播候选资料重复")
		}
		seen[candidate.ProfileID] = struct{}{}
		values = append(values, "(CAST(? AS bigint),CAST(? AS bigint),CAST(? AS bigint))")
		args = append(args, candidate.ProfileID, candidate.AccountID, candidate.Order)
	}
	args = append(args, tenantID, channelID)
	result, err := tx.Ctx(ctx).Exec(`WITH requested(profile_id,account_id,old_order) AS (
		VALUES `+strings.Join(values, ",")+`
	), positioned AS MATERIALIZED (
		SELECT profile_id,account_id,old_order,nextval('hg_ybp_batch_order_seq') AS next_order
		FROM requested ORDER BY old_order,profile_id
	) UPDATE `+publishChannelProfileTable+` cp
	SET batch_order=r.next_order
	FROM positioned r
	WHERE cp.tenant_id=? AND cp.channel_id=? AND cp.profile_id=r.profile_id
	  AND cp.account_id=r.account_id AND cp.status='active' AND cp.batch_order=r.old_order
	  AND EXISTS (SELECT 1 FROM `+dao.ContentProfile.Table()+` p
	    WHERE p.id=cp.profile_id AND p.status=1 AND p.visibility='public' AND p.deleted_at IS NULL)`, args...)
	if err != nil {
		return gerror.Wrap(err, "领取批次轮播资料失败")
	}
	count, err := result.RowsAffected()
	if err != nil {
		return gerror.Wrap(err, "读取批次轮播领取数量失败")
	}
	if count != int64(len(candidates)) {
		return gerror.New("批次轮播候选已变化，请回滚事务后重试，候选数：" + strconv.Itoa(len(candidates)))
	}
	return nil
}
