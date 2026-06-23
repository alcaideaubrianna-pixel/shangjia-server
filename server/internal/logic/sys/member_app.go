package sys

import (
	"context"
	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/input/sysin"
	"hotgo/internal/service"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/grand"
)

const (
	memberProfileActionReject = "reject"
	memberProfileActionBlock  = "block"
)

type sSysMemberApp struct{}

func NewSysMemberApp() *sSysMemberApp {
	return &sSysMemberApp{}
}

func init() {
	service.RegisterSysMemberApp(NewSysMemberApp())
}

func (s *sSysMemberApp) Settings(ctx context.Context, in *sysin.MemberSettingsInp) (res *sysin.MemberSettingsModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	return s.getOrCreateSettings(ctx, memberId)
}

func (s *sSysMemberApp) UpdateSettings(ctx context.Context, in *sysin.MemberSettingsEditInp) (err error) {
	if err = in.Filter(ctx); err != nil {
		return
	}
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if _, err = s.getOrCreateSettings(ctx, memberId); err != nil {
		return
	}

	columns := dao.MemberSetting.Columns()
	data := g.Map{
		columns.MessageEnabled:  in.MessageEnabled,
		columns.HideOnline:      in.HideOnline,
		columns.HideViewHistory: in.HideViewHistory,
		columns.MatchChatOnly:   in.MatchChatOnly,
		columns.ProfileScope:    in.ProfileScope,
		columns.PhotoScope:      in.PhotoScope,
		columns.ThemeMode:       in.ThemeMode,
	}
	_, err = dao.MemberSetting.Ctx(ctx).
		Where(columns.MemberId, memberId).
		WhereNull(columns.DeletedAt).
		Data(data).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "更新个人设置失败，请稍后重试")
		return
	}
	return
}

func (s *sSysMemberApp) FavoriteList(ctx context.Context, in *sysin.MemberFavoriteListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	favColumns := dao.MemberFavorite.Columns()
	return s.listProfilesByMemberAction(ctx, dao.MemberFavorite.Table(), "f", favColumns.MemberId, favColumns.ProfileId, favColumns.DeletedAt, favColumns.CreatedAt, memberId, in.Page, in.PerPage)
}

func (s *sSysMemberApp) BlockedProfileList(ctx context.Context, in *sysin.MemberBlockedProfileListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	columns := dao.MemberProfileAction.Columns()
	return s.listProfilesByMemberActionWithWhere(ctx, dao.MemberProfileAction.Table(), "a", columns.MemberId, columns.ProfileId, columns.DeletedAt, columns.CreatedAt, memberId, in.Page, in.PerPage, g.Map{
		aliasField("a", columns.ActionType): memberProfileActionBlock,
	})
}

func (s *sSysMemberApp) listProfilesByMemberAction(ctx context.Context, table string, alias string, memberColumn string, profileColumn string, deletedColumn string, orderColumn string, memberId int64, page int, perPage int) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	return s.listProfilesByMemberActionWithWhere(ctx, table, alias, memberColumn, profileColumn, deletedColumn, orderColumn, memberId, page, perPage, nil)
}

func (s *sSysMemberApp) listProfilesByMemberActionWithWhere(ctx context.Context, table string, alias string, memberColumn string, profileColumn string, deletedColumn string, orderColumn string, memberId int64, page int, perPage int, extraWhere g.Map) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	profileColumns := dao.ContentProfile.Columns()
	mod := g.DB().Model(table).Safe().Ctx(ctx).As(alias).
		LeftJoin(dao.ContentProfile.Table()+" p", aliasField("p", profileColumns.Id)+"="+aliasField(alias, profileColumn)).
		Where(aliasField(alias, memberColumn), memberId).
		WhereNull(aliasField(alias, deletedColumn)).
		Where(aliasField("p", profileColumns.Status), consts.StatusEnabled).
		Where(aliasField("p", profileColumns.ReviewStatus), consts.ContentReviewApproved).
		WhereIn(aliasField("p", profileColumns.Visibility), []string{consts.ContentVisibilityPublic, consts.ContentVisibilityMemberOnly})
	if len(extraWhere) > 0 {
		mod = mod.Where(extraWhere)
	}

	totalCount, err = mod.Count()
	if err != nil {
		err = gerror.Wrap(err, "获取收藏数据行失败")
		return
	}
	if totalCount == 0 {
		list = []*sysin.ContentProfileListModel{}
		return
	}

	mod = mod.Fields(
		aliasFields("p",
			profileColumns.Id,
			profileColumns.ProfileNo,
			profileColumns.Title,
			profileColumns.Summary,
			profileColumns.Province,
			profileColumns.City,
			profileColumns.Age,
			profileColumns.Height,
			profileColumns.Weight,
			profileColumns.CupSize,
			profileColumns.HasVerificationVideo,
			profileColumns.MemberOnlyVideo,
			profileColumns.ImageCount,
			profileColumns.VideoCount,
			profileColumns.PublishedAt,
		),
		aliasField(alias, orderColumn)+" AS action_at",
	).
		Page(page, perPage).
		OrderDesc(aliasField(alias, orderColumn)).
		OrderDesc(aliasField(alias, "id"))

	var rows []contentProfileRow
	if err = mod.Scan(&rows); err != nil {
		err = gerror.Wrap(err, "获取收藏列表失败，请稍后重试")
		return
	}

	profileIds := make([]int64, 0, len(rows))
	for _, row := range rows {
		profileIds = append(profileIds, row.Id)
	}
	contentService := NewSysContent()
	coverMap, err := contentService.getProfileCoverMap(ctx, profileIds)
	if err != nil {
		return
	}
	mediaMap, err := contentService.getProfileMediaMap(ctx, profileIds, contentService.isRequestVip(ctx))
	if err != nil {
		return
	}
	list = make([]*sysin.ContentProfileListModel, 0, len(rows))
	for _, row := range rows {
		item := row.toListModel()
		item.ActionAt = row.ActionAt
		item.CoverUrl = coverMap[row.Id]
		item.Avatar = item.CoverUrl
		item.Media = mediaMap[row.Id]
		item.Photos = mediaPhotos(item.Media)
		list = append(list, item)
	}
	return
}

func (s *sSysMemberApp) FavoriteToggle(ctx context.Context, in *sysin.MemberFavoriteToggleInp) (res *sysin.MemberFavoriteToggleModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	profileColumns := dao.ContentProfile.Columns()
	count, err := dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, in.ProfileId).
		Where(profileColumns.Status, consts.StatusEnabled).
		Where(profileColumns.ReviewStatus, consts.ContentReviewApproved).
		WhereNull(profileColumns.DeletedAt).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "验证资料失败，请稍后重试")
		return
	}
	if count == 0 {
		err = gerror.New("资料不存在或暂未公开")
		return
	}

	favColumns := dao.MemberFavorite.Columns()
	var existing gdb.Record
	if existing, err = dao.MemberFavorite.Ctx(ctx).
		Where(favColumns.MemberId, memberId).
		Where(favColumns.ProfileId, in.ProfileId).
		WhereNull(favColumns.DeletedAt).
		One(); err != nil {
		err = gerror.Wrap(err, "读取收藏状态失败，请稍后重试")
		return
	}
	if existing != nil {
		_, err = dao.MemberFavorite.Ctx(ctx).
			WherePri(existing[favColumns.Id].Int64()).
			Data(g.Map{favColumns.DeletedAt: gtime.Now()}).
			Update()
		if err != nil {
			err = gerror.Wrap(err, "取消收藏失败，请稍后重试")
			return
		}
		res = &sysin.MemberFavoriteToggleModel{Favorited: false}
		return
	}
	_, err = dao.MemberFavorite.Ctx(ctx).Data(g.Map{
		favColumns.MemberId:  memberId,
		favColumns.ProfileId: in.ProfileId,
	}).Insert()
	if err != nil {
		err = gerror.Wrap(err, "收藏失败，请稍后重试")
		return
	}
	res = &sysin.MemberFavoriteToggleModel{Favorited: true}
	return
}

func (s *sSysMemberApp) FavoriteIds(ctx context.Context, in *sysin.MemberFavoriteIdsInp) (res *sysin.MemberFavoriteIdsModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	columns := dao.MemberFavorite.Columns()
	values, err := dao.MemberFavorite.Ctx(ctx).
		Fields(columns.ProfileId).
		Where(columns.MemberId, memberId).
		WhereNull(columns.DeletedAt).
		Array()
	if err != nil {
		err = gerror.Wrap(err, "获取收藏状态失败，请稍后重试")
		return
	}
	res = &sysin.MemberFavoriteIdsModel{Ids: make([]int64, 0, len(values))}
	for _, value := range values {
		res.Ids = append(res.Ids, value.Int64())
	}
	return
}

func (s *sSysMemberApp) ProfileRelation(ctx context.Context, in *sysin.MemberProfileRelationInp) (res *sysin.MemberProfileRelationModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if err = s.verifyProfileVisible(ctx, in.ProfileId); err != nil {
		return
	}
	res = &sysin.MemberProfileRelationModel{}
	favColumns := dao.MemberFavorite.Columns()
	favoriteCount, err := dao.MemberFavorite.Ctx(ctx).
		Where(favColumns.MemberId, memberId).
		Where(favColumns.ProfileId, in.ProfileId).
		WhereNull(favColumns.DeletedAt).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "读取喜欢状态失败，请稍后重试")
		return
	}
	res.Favorited = favoriteCount > 0

	actionColumns := dao.MemberProfileAction.Columns()
	records, err := dao.MemberProfileAction.Ctx(ctx).
		Fields(actionColumns.ActionType).
		Where(actionColumns.MemberId, memberId).
		Where(actionColumns.ProfileId, in.ProfileId).
		WhereIn(actionColumns.ActionType, []string{memberProfileActionBlock, memberProfileActionReject}).
		WhereNull(actionColumns.DeletedAt).
		Array()
	if err != nil {
		err = gerror.Wrap(err, "读取资料关系失败，请稍后重试")
		return
	}
	for _, record := range records {
		switch record.String() {
		case memberProfileActionBlock:
			res.Blocked = true
		case memberProfileActionReject:
			res.Rejected = true
		}
	}
	return
}

func (s *sSysMemberApp) BlockProfile(ctx context.Context, in *sysin.MemberProfileActionInp) (err error) {
	return s.saveProfileAction(ctx, in.ProfileId, memberProfileActionBlock)
}

func (s *sSysMemberApp) UnblockProfile(ctx context.Context, in *sysin.MemberProfileActionInp) (err error) {
	return s.removeProfileAction(ctx, in.ProfileId, memberProfileActionBlock)
}

func (s *sSysMemberApp) RejectProfile(ctx context.Context, in *sysin.MemberProfileActionInp) (err error) {
	return s.saveProfileAction(ctx, in.ProfileId, memberProfileActionReject)
}

func (s *sSysMemberApp) ImmersiveProfileList(ctx context.Context, in *sysin.MemberImmersiveProfileListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	if _, err = s.loginMemberId(ctx); err != nil {
		return
	}
	listInp := in.ContentProfileListInp
	listInp.ExcludeActions = []string{memberProfileActionBlock, memberProfileActionReject}
	return service.SysContent().ListProfiles(ctx, &listInp)
}

func (s *sSysMemberApp) TraceList(ctx context.Context, in *sysin.MemberProfileTraceListInp) (list []*sysin.ContentProfileListModel, totalCount int, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	columns := dao.MemberProfileView.Columns()
	return s.listProfilesByMemberAction(ctx, dao.MemberProfileView.Table(), "v", columns.MemberId, columns.ProfileId, columns.DeletedAt, columns.LastViewAt, memberId, in.Page, in.PerPage)
}

func (s *sSysMemberApp) saveProfileAction(ctx context.Context, profileId int64, actionType string) (err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if err = s.verifyProfileVisible(ctx, profileId); err != nil {
		return
	}
	columns := dao.MemberProfileAction.Columns()
	var existing gdb.Record
	if existing, err = dao.MemberProfileAction.Ctx(ctx).
		Where(columns.MemberId, memberId).
		Where(columns.ProfileId, profileId).
		Where(columns.ActionType, actionType).
		WhereNull(columns.DeletedAt).
		One(); err != nil {
		err = gerror.Wrap(err, "读取资料动作失败，请稍后重试")
		return
	}
	if existing != nil {
		return nil
	}
	_, err = dao.MemberProfileAction.Ctx(ctx).Data(g.Map{
		columns.MemberId:   memberId,
		columns.ProfileId:  profileId,
		columns.ActionType: actionType,
	}).Insert()
	if err != nil {
		err = gerror.Wrap(err, "保存资料动作失败，请稍后重试")
	}
	return
}

func (s *sSysMemberApp) removeProfileAction(ctx context.Context, profileId int64, actionType string) (err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if err = s.verifyProfileVisible(ctx, profileId); err != nil {
		return
	}
	columns := dao.MemberProfileAction.Columns()
	_, err = dao.MemberProfileAction.Ctx(ctx).
		Where(columns.MemberId, memberId).
		Where(columns.ProfileId, profileId).
		Where(columns.ActionType, actionType).
		WhereNull(columns.DeletedAt).
		Data(g.Map{columns.DeletedAt: gtime.Now()}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "解除资料拉黑失败，请稍后重试")
	}
	return
}

func (s *sSysMemberApp) TraceRecord(ctx context.Context, in *sysin.MemberProfileTraceRecordInp) (err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if err = s.verifyProfileVisible(ctx, in.ProfileId); err != nil {
		return
	}
	columns := dao.MemberProfileView.Columns()
	var existing gdb.Record
	if existing, err = dao.MemberProfileView.Ctx(ctx).
		Where(columns.MemberId, memberId).
		Where(columns.ProfileId, in.ProfileId).
		WhereNull(columns.DeletedAt).
		One(); err != nil {
		err = gerror.Wrap(err, "读取浏览痕迹失败，请稍后重试")
		return
	}
	if existing != nil {
		_, err = dao.MemberProfileView.Ctx(ctx).
			WherePri(existing[columns.Id].Int64()).
			Data(g.Map{
				columns.ViewCount:  gdb.Raw(columns.ViewCount + "+1"),
				columns.LastViewAt: gtime.Now(),
			}).
			Update()
		if err != nil {
			err = gerror.Wrap(err, "更新浏览痕迹失败，请稍后重试")
		}
		if err = s.increaseProfileStats(ctx, in.ProfileId); err != nil {
			err = gerror.Wrap(err, "更新资料热度失败，请稍后重试")
		}
		return
	}
	_, err = dao.MemberProfileView.Ctx(ctx).Data(g.Map{
		columns.MemberId:   memberId,
		columns.ProfileId:  in.ProfileId,
		columns.ViewCount:  1,
		columns.LastViewAt: gtime.Now(),
	}).Insert()
	if err != nil {
		err = gerror.Wrap(err, "记录浏览痕迹失败，请稍后重试")
		return
	}
	if err = s.increaseProfileStats(ctx, in.ProfileId); err != nil {
		err = gerror.Wrap(err, "更新资料热度失败，请稍后重试")
	}
	return
}

func (s *sSysMemberApp) increaseProfileStats(ctx context.Context, profileId int64) error {
	if profileId <= 0 {
		return nil
	}
	now := gtime.Now().String()
	if isPgsql() {
		_, err := g.DB().Exec(ctx, `
INSERT INTO hg_content_profile_stats (profile_id, view_total, view_24h, click_total, hot_score, last_view_at, created_at, updated_at)
VALUES (?, 1, 1, 0, 1, ?, ?, ?)
ON CONFLICT (profile_id) DO UPDATE SET
  view_total = hg_content_profile_stats.view_total + 1,
  view_24h = hg_content_profile_stats.view_24h + 1,
  hot_score = hg_content_profile_stats.hot_score + 1,
  last_view_at = EXCLUDED.last_view_at,
  updated_at = EXCLUDED.updated_at`, profileId, now, now, now)
		return err
	}
	_, err := g.DB().Exec(ctx, `
INSERT INTO hg_content_profile_stats (profile_id, view_total, view_24h, click_total, hot_score, last_view_at, created_at, updated_at)
VALUES (?, 1, 1, 0, 1, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  view_total = view_total + 1,
  view_24h = view_24h + 1,
  hot_score = hot_score + 1,
  last_view_at = VALUES(last_view_at),
  updated_at = VALUES(updated_at)`, profileId, now, now, now)
	return err
}

func (s *sSysMemberApp) Stats(ctx context.Context, in *sysin.MemberStatsInp) (res *sysin.MemberStatsModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	columns := dao.MemberFavorite.Columns()
	favoriteCount, err := dao.MemberFavorite.Ctx(ctx).
		Where(columns.MemberId, memberId).
		WhereNull(columns.DeletedAt).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "获取个人统计失败，请稍后重试")
		return
	}
	traceColumns := dao.MemberProfileView.Columns()
	traceCount, err := dao.MemberProfileView.Ctx(ctx).
		Where(traceColumns.MemberId, memberId).
		WhereNull(traceColumns.DeletedAt).
		Count()
	if err != nil {
		err = gerror.Wrap(err, "获取浏览痕迹统计失败，请稍后重试")
		return
	}
	res = &sysin.MemberStatsModel{
		FavoriteCount: favoriteCount,
		ContactCount:  0,
		TraceCount:    traceCount,
		MatchCount:    traceCount,
	}
	return
}

func (s *sSysMemberApp) Agreement(ctx context.Context, in *sysin.MemberAgreementInp) (res *sysin.MemberAgreementModel, err error) {
	res = &sysin.MemberAgreementModel{Type: in.Type}
	switch in.Type {
	case "privacy":
		res.Title = "隐私政策"
		res.Content = "我们仅收集提供服务所必需的信息，并会按照法律法规和平台规则保护你的个人信息。"
	case "user":
		res.Title = "用户协议"
		res.Content = "使用悦伴服务前，请确认你已了解账号、安全、内容访问和会员权益相关规则。"
	default:
		err = gerror.New("协议类型不存在")
	}
	return
}

func (s *sSysMemberApp) CreateShare(ctx context.Context, in *sysin.MemberShareCreateInp) (res *sysin.MemberShareCreateModel, err error) {
	memberId, err := s.loginMemberId(ctx)
	if err != nil {
		return
	}
	if err = s.verifyProfileVisible(ctx, in.ProfileId); err != nil {
		return
	}

	columns := dao.MemberShare.Columns()
	one, err := dao.MemberShare.Ctx(ctx).
		Fields(columns.ShareToken).
		Where(columns.MemberId, memberId).
		Where(columns.ProfileId, in.ProfileId).
		WhereNull(columns.DeletedAt).
		One()
	if err != nil {
		err = gerror.Wrap(err, "读取分享链接失败，请稍后重试")
		return
	}
	token := ""
	if one != nil {
		token = one[columns.ShareToken].String()
	}
	if token == "" {
		token = s.genShareToken()
		_, err = dao.MemberShare.Ctx(ctx).Data(g.Map{
			columns.MemberId:   memberId,
			columns.ProfileId:  in.ProfileId,
			columns.ShareToken: token,
		}).Insert()
		if err != nil {
			err = gerror.Wrap(err, "生成分享链接失败，请稍后重试")
			return
		}
	}
	res = &sysin.MemberShareCreateModel{
		ShareToken: token,
		ShareUrl:   "/pages/profile/detail?id=" + g.NewVar(in.ProfileId).String() + "&share=" + token,
	}
	return
}

func (s *sSysMemberApp) OpenShare(ctx context.Context, in *sysin.MemberShareOpenInp) (res *sysin.MemberShareOpenModel, err error) {
	shareColumns := dao.MemberShare.Columns()
	memberColumns := dao.AdminMember.Columns()
	var row struct {
		Id         int64  `json:"id"`
		ProfileId  int64  `json:"profileId"`
		InviteCode string `json:"inviteCode"`
	}
	if err = dao.MemberShare.Ctx(ctx).As("s").
		LeftJoin(dao.AdminMember.Table()+" m", aliasField("m", memberColumns.Id)+"="+aliasField("s", shareColumns.MemberId)).
		Fields(
			aliasField("s", shareColumns.Id),
			aliasField("s", shareColumns.ProfileId),
			aliasField("m", memberColumns.InviteCode)+" AS inviteCode",
		).
		Where(aliasField("s", shareColumns.ShareToken), in.ShareToken).
		WhereNull(aliasField("s", shareColumns.DeletedAt)).
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "读取分享链接失败，请稍后重试")
		return
	}
	if row.Id <= 0 {
		err = gerror.New("分享链接不存在或已失效")
		return
	}
	_, err = dao.MemberShare.Ctx(ctx).
		WherePri(row.Id).
		Data(g.Map{
			shareColumns.VisitCount:  gdb.Raw(shareColumns.VisitCount + "+1"),
			shareColumns.LastVisitAt: gtime.Now(),
		}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "记录分享访问失败，请稍后重试")
		return
	}
	res = &sysin.MemberShareOpenModel{
		ProfileId:  row.ProfileId,
		InviteCode: row.InviteCode,
	}
	return
}

func (s *sSysMemberApp) BindShareRegister(ctx context.Context, in *sysin.MemberShareRegisterInp) (err error) {
	if in.ShareToken == "" || in.MemberId <= 0 {
		return nil
	}
	columns := dao.MemberShare.Columns()
	_, err = dao.MemberShare.Ctx(ctx).
		Where(columns.ShareToken, in.ShareToken).
		WhereNull(columns.DeletedAt).
		Data(g.Map{
			columns.RegisterCount: gdb.Raw(columns.RegisterCount + "+1"),
		}).
		Update()
	if err != nil {
		err = gerror.Wrap(err, "记录分享注册失败")
	}
	return
}

func (s *sSysMemberApp) loginMemberId(ctx context.Context) (memberId int64, err error) {
	memberId = contexts.GetUserId(ctx)
	if memberId <= 0 {
		err = gerror.New("请先登录")
	}
	return
}

func (s *sSysMemberApp) verifyProfileVisible(ctx context.Context, profileId int64) (err error) {
	profileColumns := dao.ContentProfile.Columns()
	count, err := dao.ContentProfile.Ctx(ctx).
		Where(profileColumns.Id, profileId).
		Where(profileColumns.Status, consts.StatusEnabled).
		Where(profileColumns.ReviewStatus, consts.ContentReviewApproved).
		WhereNull(profileColumns.DeletedAt).
		Count()
	if err != nil {
		return gerror.Wrap(err, "验证资料失败，请稍后重试")
	}
	if count == 0 {
		return gerror.New("资料不存在或暂未公开")
	}
	return nil
}

func (s *sSysMemberApp) genShareToken() string {
	return grand.S(32)
}

func (s *sSysMemberApp) getOrCreateSettings(ctx context.Context, memberId int64) (res *sysin.MemberSettingsModel, err error) {
	columns := dao.MemberSetting.Columns()
	var row *sysin.MemberSettingsModel
	if err = dao.MemberSetting.Ctx(ctx).
		Fields(
			columns.MessageEnabled,
			columns.HideOnline,
			columns.HideViewHistory,
			columns.MatchChatOnly,
			columns.ProfileScope,
			columns.PhotoScope,
			columns.ThemeMode,
		).
		Where(columns.MemberId, memberId).
		WhereNull(columns.DeletedAt).
		Scan(&row); err != nil {
		err = gerror.Wrap(err, "读取个人设置失败，请稍后重试")
		return
	}
	if row != nil {
		return row, nil
	}
	res = defaultMemberSettings()
	_, err = dao.MemberSetting.Ctx(ctx).Data(g.Map{
		columns.MemberId:        memberId,
		columns.MessageEnabled:  res.MessageEnabled,
		columns.HideOnline:      res.HideOnline,
		columns.HideViewHistory: res.HideViewHistory,
		columns.MatchChatOnly:   res.MatchChatOnly,
		columns.ProfileScope:    res.ProfileScope,
		columns.PhotoScope:      res.PhotoScope,
		columns.ThemeMode:       res.ThemeMode,
	}).Insert()
	if err != nil {
		err = gerror.Wrap(err, "初始化个人设置失败，请稍后重试")
	}
	return
}

func defaultMemberSettings() *sysin.MemberSettingsModel {
	return &sysin.MemberSettingsModel{
		MessageEnabled:  1,
		HideOnline:      0,
		HideViewHistory: 1,
		MatchChatOnly:   1,
		ProfileScope:    "all",
		PhotoScope:      "verified",
		ThemeMode:       sysin.MemberThemeSystem,
	}
}
