package sys

import (
	"context"
	"strconv"
	"strings"

	"hotgo/internal/dao"
	"hotgo/internal/library/profilescope"
	"hotgo/internal/model/input/sysin"
)

func supportsOpenProfileCounter(in *sysin.ContentProfileListInp) bool {
	province := strings.TrimSpace(in.Province)
	city := strings.TrimSpace(in.City)
	return len(in.ProfileIds) == 0 && in.Provinces == "" && strings.TrimSpace(in.Keyword) == "" &&
		(in.Province == "" || province != "") && (in.City == "" || city != "" && province != "") &&
		in.AgeRanges == "" && in.HeightRanges == "" && in.WeightRanges == "" &&
		in.AgeMin <= 0 && in.HeightMin <= 0 && in.WeightMin <= 0 &&
		in.Cup == "" && in.Cups == "" && in.HasVideo != 1 && in.HasVerification != 1 &&
		in.CanFly != 1 && in.CanGoAbroad != 1 && in.CanOvernight != 1 && in.CanCohabitate != 1 &&
		in.HasHealthCheck != 1 && in.IsFullMonth != 1 && in.IsVirgin != 1 && in.AcceptSm != 1 &&
		in.NoCondom != 1 && in.AllowCreampie != 1 && in.HasTattoo != 1
}

func (s *sSysContent) openProfileCounter(ctx context.Context, in *sysin.ContentProfileListInp, memberID int64) (int, error) {
	excluded := parseInt64FilterValues(in.ExcludeProfileIds)
	if memberID > 0 {
		columns := dao.MemberProfileAction.Columns()
		ids, err := dao.MemberProfileAction.Ctx(ctx).
			Fields(columns.ProfileId).
			Where(columns.MemberId, memberID).
			WhereIn(columns.ActionType, in.ExcludeActions).
			WhereNull(columns.DeletedAt).
			Array()
		if err != nil {
			return 0, err
		}
		for _, id := range ids {
			// Match the list query's NOT IN semantics if a legacy action has NULL.
			if id.IsNil() {
				return 0, nil
			}
			excluded = append(excluded, id.Int64())
		}
	}
	excludedValues := make([]string, 0, len(excluded))
	for _, id := range excluded {
		excludedValues = append(excludedValues, strconv.FormatInt(id, 10))
	}

	scope := profilescope.FromContext(ctx)
	value, err := dao.ContentProfile.DB().GetValue(ctx, `
		SELECT public.open_profile_exact_count(
			?, ?, NULLIF(?, ''), NULLIF(?, ''), string_to_array(?, ',')::bigint[]
		)`, scope.TenantIds[0], scope.RequireEnabledAccount,
		strings.TrimSpace(in.Province), strings.TrimSpace(in.City), strings.Join(excludedValues, ","))
	if err != nil {
		return 0, err
	}
	return value.Int(), nil
}
