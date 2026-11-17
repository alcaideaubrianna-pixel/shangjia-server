package sys

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/dao"
)

// Only the branch prefix participates: group/message identities deliberately
// do not, so repeated material within one source/rule still deduplicates.
func collectProfileFingerprintNamespace(sourceKey string) string {
	parts := strings.SplitN(sourceKey, ":", 9)
	if len(parts) != 9 || parts[0] != "collect" || parts[1] != "v4" || parts[4] != "source" || parts[6] != "rule" || parts[8] == "" {
		return ""
	}
	for _, index := range []int{2, 3, 5, 7} {
		id, err := strconv.ParseInt(parts[index], 10, 64)
		if err != nil || id <= 0 || strconv.FormatInt(id, 10) != parts[index] {
			return ""
		}
	}
	return strings.Join(parts[:8], ":")
}

// scopeCollectProfileFingerprints accepts RAW buildProfileFingerprints output.
// Always return a copy: callers retain raw signatures for legacy lookup. Apply
// once per generation boundary; this is not a rehash of stored fingerprints.
// Only exact index signatures change; media pHash bits are never modified.
func scopeCollectProfileFingerprints(sourceKey string, items []profileFingerprint) []profileFingerprint {
	if items == nil {
		return nil
	}
	result := make([]profileFingerprint, len(items))
	copy(result, items)
	namespace := collectProfileFingerprintNamespace(sourceKey)
	if namespace == "" {
		return result
	}
	for index := range result {
		digest := sha256.Sum256([]byte(namespace + "\x00" + result[index].Signature))
		// Fits the existing varchar(64), with no schema/index changes.
		result[index].Signature = hex.EncodeToString(digest[:])
	}
	return result
}

// findLegacyCollectRuleDuplicate queries raw exact fingerprints only for old
// non-v4 profiles with explicit same-branch dispatch evidence. It does not
// rewrite old rows, warm cross-branch cache entries, or trigger a backfill.
func (s *sSysPublish) findLegacyCollectRuleDuplicate(ctx context.Context, event, rule gdb.Record, items []profileFingerprint) (*profileFingerprintDuplicateError, error) {
	if len(items) == 0 {
		return nil, nil
	}
	tenantID, accountID := event["tenant_id"].Int64(), event["account_id"].Int64()
	sourceID, ruleID := event["source_id"].Int64(), rule["id"].Int64()
	if tenantID <= 0 || accountID <= 0 || sourceID <= 0 || ruleID <= 0 || collectRuleSourceID(event, rule) != sourceID {
		return nil, gerror.New("旧版采集指纹查询缺少同来源同规则归属")
	}
	accountScope := collectEventAccountScope(event)
	const fingerprintLimit = 256
	if len(items) > fingerprintLimit {
		return nil, gerror.New("旧版采集指纹查询超过安全上限")
	}
	conditions := make([]string, 0, len(items))
	args := make([]interface{}, 0, len(items)*5)
	for _, item := range items {
		if item.ChannelID <= 0 || item.Layer == "" || item.Layer == "_indexed" || item.Signature == "" {
			continue
		}
		conditions = append(conditions, "(f.channel_id=? AND f.layer=? AND f.signature=? AND f.item_total=? AND f.signature_count=?)")
		args = append(args, item.ChannelID, item.Layer, item.Signature, item.ItemTotal, item.SignatureCount)
	}
	if len(conditions) == 0 {
		return nil, nil
	}
	parentCtx := ctx
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if tx := gdb.TXFromCtx(parentCtx, g.DB().GetGroup()); tx != nil {
		defer tx.Ctx(parentCtx)
	}
	// Include exact non-owner associations: legacy global owner uniqueness
	// could leave this branch's own profile as an association of another owner.
	// Historical fingerprints/branch evidence may retain transport account 0.
	// The profile-state predicate below still requires the actual business
	// owner; raw account 0 is never accepted as proof of profile ownership.
	// Both dispatch predicates are profile-index lookups after exact key match.
	row, err := g.DB().Model(publishProfileFingerprintTable+" f").Safe().Ctx(ctx).
		InnerJoin(dao.ContentProfile.Table()+" p", "p.id=f.profile_id AND p.deleted_at IS NULL").
		Fields("f.profile_id,f.channel_id,f.layer").
		Where("f.tenant_id", tenantID).WhereIn("f.account_id", accountScope).
		Where("("+strings.Join(conditions, " OR ")+")", args...).
		Where("(p.source_key IS NULL OR p.source_key NOT LIKE ?)", "collect:v4:%").
		Where("EXISTS (SELECT 1 FROM "+publishProfileStateTable+" ps WHERE ps.profile_id=p.id AND ps.tenant_id=? AND ps.account_id=? AND ps.deleted_at IS NULL)", tenantID, accountID).
		Where("EXISTS (SELECT 1 FROM "+publishCollectDispatchTable+" d WHERE d.profile_id=f.profile_id AND d.tenant_id=? AND d.account_id IN (?) AND d.source_id=? AND d.rule_id=?)", tenantID, accountScope, sourceID, ruleID).
		Where("NOT EXISTS (SELECT 1 FROM "+publishCollectDispatchTable+" other WHERE other.profile_id=f.profile_id AND (other.tenant_id<>? OR other.account_id NOT IN (?) OR other.source_id<>? OR other.rule_id<>?))", tenantID, accountScope, sourceID, ruleID).
		Where("NOT EXISTS (SELECT 1 FROM "+publishCollectChannelProfileTable+" cp WHERE cp.profile_id=f.profile_id AND (cp.tenant_id<>? OR cp.account_id NOT IN (?) OR cp.source_id<>? OR cp.rule_id<>?))", tenantID, accountScope, sourceID, ruleID).
		OrderDesc("f.profile_id").Limit(1).One()
	if err != nil {
		return nil, gerror.Wrap(err, "查询同来源同规则旧版采集指纹失败")
	}
	if row.IsEmpty() {
		return nil, nil
	}
	return &profileFingerprintDuplicateError{
		ProfileID: row["profile_id"].Int64(), ChannelID: row["channel_id"].Int64(), Layer: row["layer"].String(),
	}, nil
}
