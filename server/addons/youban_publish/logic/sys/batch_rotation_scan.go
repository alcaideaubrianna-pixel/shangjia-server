package sys

import (
	"context"
	"errors"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
)

var errBatchRotationScanBudget = errors.New("频道轮播候选扫描达到本次预算，保留进度后稍后继续")

func batchRotationCandidatePageSQL() string {
	return `WITH page AS MATERIALIZED (
		SELECT profile_id,account_id,batch_order,tenant_id,channel_id
		FROM ` + publishChannelProfileTable + `
		WHERE tenant_id=? AND channel_id=? AND status='active' AND batch_order>0 AND cycle_override_enabled=false
		  AND (batch_order,profile_id)>(?,?)
		  AND (batch_order,profile_id)<=(?,?)
		ORDER BY batch_order,profile_id LIMIT ?
	) SELECT cp.profile_id,cp.account_id,cp.batch_order,
	    (` + batchRotationEligibilitySQL() + `) AS eligible
	  FROM page cp ORDER BY cp.batch_order,cp.profile_id`
}

// Check the usable index, including its partial predicate. A similarly named
// but invalid or differently scoped index must not enable an unindexed reader.
func batchRotationIndexReadyTx(ctx context.Context, tx gdb.TX) error {
	row, err := tx.Ctx(ctx).GetOne(`SELECT EXISTS (
		SELECT 1 FROM pg_index i
		WHERE i.indrelid=to_regclass(?) AND i.indexrelid=to_regclass('idx_ybp_batch_inherited_rotation')
		  AND i.indisvalid AND i.indisready AND i.indnkeyatts=5
		  AND lower(regexp_replace(pg_get_expr(i.indpred,i.indrelid),
		    '::(text|character varying)|[()[:space:]]', '', 'g'))='status=''active''andbatch_order>0andcycle_override_enabled=false'
		  AND ARRAY(SELECT a.attname::text
		    FROM unnest(i.indkey::smallint[]) WITH ORDINALITY k(attnum,n)
		    JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum
		    WHERE k.n<=5 ORDER BY k.n)=
		    ARRAY['tenant_id','channel_id','status','batch_order','profile_id']
	) AS ready`, publishChannelProfileTable)
	if err != nil {
		return gerror.Wrap(err, "检查频道轮播读取索引失败")
	}
	if !row["ready"].Bool() {
		return gerror.New("频道轮播迁移未就绪：缺少有效的 idx_ybp_batch_inherited_rotation 索引")
	}
	return nil
}

// Ineligible projection rows are rotated rather than permanently deactivated.
// A later permission/configuration change can make them eligible again without
// requiring a full rebuild. CAS preserves concurrent admission/success writes.
// The caller commits this progress on errBatchRotationScanBudget, without
// consuming the day's snapshot or pretending any publication succeeded.
func rotateBatchRotationRejectedTx(ctx context.Context, tx gdb.TX, tenantID, channelID int64, rows []batchRotationCandidate) error {
	if len(rows) == 0 {
		return nil
	}
	if len(rows) > 500 {
		return gerror.New("单页轮播资格修复数量超过上限")
	}
	values := make([]string, 0, len(rows))
	args := make([]interface{}, 0, 3*len(rows)+2)
	for _, row := range rows {
		values = append(values, "(CAST(? AS bigint),CAST(? AS bigint),CAST(? AS bigint))")
		args = append(args, row.ProfileID, row.AccountID, row.Order)
	}
	args = append(args, tenantID, channelID)
	_, err := tx.Ctx(ctx).Exec(`WITH requested(profile_id,account_id,old_order) AS (
		VALUES `+strings.Join(values, ",")+`
	), positioned AS MATERIALIZED (
		SELECT profile_id,account_id,old_order,nextval('hg_ybp_batch_order_seq') AS next_order
		FROM requested ORDER BY old_order,profile_id
	) UPDATE `+publishChannelProfileTable+` cp SET batch_order=r.next_order
	FROM positioned r
	WHERE cp.tenant_id=? AND cp.channel_id=? AND cp.profile_id=r.profile_id
	  AND cp.account_id=r.account_id AND cp.status='active' AND cp.batch_order=r.old_order`, args...)
	if err != nil {
		return gerror.Wrap(err, "推进失效轮播候选扫描位置失败")
	}
	return nil
}
