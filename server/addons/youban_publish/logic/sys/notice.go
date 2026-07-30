package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	pdao "hotgo/addons/youban_publish/internal/dao"
	pentity "hotgo/addons/youban_publish/internal/model/entity"
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/consts"
	"hotgo/internal/library/contexts"
	"hotgo/internal/model/entity"
	"hotgo/internal/model/input/adminin"
	"hotgo/internal/model/input/form"
	"hotgo/internal/websocket"
	"hotgo/utility/simple"
	"hotgo/utility/validate"
)

func (s *sSysPublish) NoticeList(ctx context.Context, in *sysin.NoticeListInp) (list []*adminin.NoticeListModel, totalCount int, err error) {
	cols := pdao.YoubanPublishNotice.Columns()
	mod := pdao.YoubanPublishNotice.Ctx(ctx).WhereNull(cols.DeletedAt)
	if in.Title != "" {
		mod = mod.WhereLike(cols.Title, "%"+in.Title+"%")
	}
	if in.Content != "" {
		mod = mod.WhereLike(cols.Content, "%"+in.Content+"%")
	}
	if in.Type > 0 {
		mod = mod.Where(cols.Type, in.Type)
	}
	if in.Status > 0 {
		mod = mod.Where(cols.Status, in.Status)
	}
	totalCount, err = mod.Clone().Count()
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取上架通知公告总数失败")
	}
	if totalCount == 0 {
		return []*adminin.NoticeListModel{}, 0, nil
	}
	var rows []*pentity.YoubanPublishNotice
	err = mod.Fields(cols.Id, cols.Type, cols.Title, cols.Content, cols.Tag, cols.Receiver, cols.Remark, cols.Sort, cols.Status, cols.PublishAt, cols.ExpireAt, cols.CreatedBy, cols.UpdatedBy, cols.CreatedAt, cols.UpdatedAt, cols.DeletedAt).
		Page(in.Page, in.PerPage).OrderDesc(cols.Sort).OrderDesc(cols.Id).Scan(&rows)
	if err != nil {
		return nil, 0, gerror.Wrap(err, "获取上架通知公告列表失败")
	}
	list = make([]*adminin.NoticeListModel, 0, len(rows))
	for _, row := range rows {
		item := noticeListModel(row)
		item.ReceiverGroup, err = s.noticeReceiverGroup(ctx, parseReceiver(row.Receiver))
		if err != nil {
			return nil, 0, err
		}
		item.ReadCount, err = pdao.YoubanPublishNoticeRead.Ctx(ctx).
			Where(pdao.YoubanPublishNoticeRead.Columns().NoticeId, row.Id).Sum(pdao.YoubanPublishNoticeRead.Columns().Clicks)
		if err != nil {
			return nil, 0, gerror.Wrap(err, "获取上架通知公告阅读数失败")
		}
		list = append(list, item)
	}
	return list, totalCount, nil
}

func (s *sSysPublish) NoticeView(ctx context.Context, in *sysin.NoticeViewInp) (res *adminin.NoticeViewModel, err error) {
	var row *pentity.YoubanPublishNotice
	err = pdao.YoubanPublishNotice.Ctx(ctx).Where(pdao.YoubanPublishNotice.Columns().Id, in.Id).WhereNull(pdao.YoubanPublishNotice.Columns().DeletedAt).Scan(&row)
	if err != nil {
		return nil, gerror.Wrap(err, "获取上架通知公告详情失败")
	}
	if row == nil || row.Id <= 0 {
		return nil, gerror.New("上架通知公告不存在")
	}
	return &adminin.NoticeViewModel{AdminNotice: noticeEntity(row)}, nil
}

func (s *sSysPublish) NoticeEdit(ctx context.Context, in *sysin.NoticeEditInp) (err error) {
	member := contexts.Get(ctx).User
	if member == nil {
		return gerror.New("获取用户信息失败")
	}
	if strings.TrimSpace(in.Title) == "" {
		return gerror.New("标题不能为空")
	}
	if strings.TrimSpace(in.Content) == "" {
		return gerror.New("请输入消息内容")
	}
	if !validate.InSlice([]int64{consts.NoticeTypeNotify, consts.NoticeTypeNotice, consts.NoticeTypeLetter}, in.Type) {
		return gerror.New("公告类型不支持")
	}
	if in.Type == consts.NoticeTypeLetter {
		if len(in.Receiver) == 0 {
			return gerror.New("私信类型必须选择接收人")
		}
		if err = s.validateNoticeReceivers(ctx, in.Receiver); err != nil {
			return err
		}
	}
	cols := pdao.YoubanPublishNotice.Columns()
	data := g.Map{
		cols.Type:      in.Type,
		cols.Title:     in.Title,
		cols.Content:   in.Content,
		cols.Tag:       in.Tag,
		cols.Receiver:  gjson.New(in.Receiver).String(),
		cols.Remark:    in.Remark,
		cols.Sort:      in.Sort,
		cols.Status:    in.Status,
		cols.PublishAt: in.PublishAt,
		cols.ExpireAt:  in.ExpireAt,
	}
	if in.Id > 0 {
		data[cols.UpdatedBy] = member.Id
		_, err = pdao.YoubanPublishNotice.Ctx(ctx).Where(cols.Id, in.Id).WhereNull(cols.DeletedAt).Data(data).Update()
		if err != nil {
			return gerror.Wrap(err, "修改上架通知公告失败")
		}
		return nil
	}
	data[cols.CreatedBy] = member.Id
	data[cols.CreatedAt] = gtime.Now()
	in.Id, err = pdao.YoubanPublishNotice.Ctx(ctx).Data(data).InsertAndGetId()
	if err != nil {
		return gerror.Wrap(err, "新增上架通知公告失败")
	}
	if !noticeShouldPushNow(in) {
		return nil
	}
	response := &websocket.WResponse{Event: "notice", Data: map[string]interface{}{
		"id": in.Id, "type": in.Type, "title": in.Title, "content": in.Content, "tag": in.Tag, "sort": in.Sort, "createdBy": member.Id, "createdAt": gtime.Now(),
	}}
	simple.SafeGo(ctx, func(ctx context.Context) {
		if in.Type == consts.NoticeTypeLetter {
			for _, receiverID := range in.Receiver {
				websocket.SendToUser(receiverID, response)
			}
			return
		}
		websocket.SendToAll(response)
	})
	return nil
}

func (s *sSysPublish) NoticeDelete(ctx context.Context, in *sysin.NoticeDeleteInp) (err error) {
	cols := pdao.YoubanPublishNotice.Columns()
	_, err = pdao.YoubanPublishNotice.Ctx(ctx).WhereIn(cols.Id, g.NewVar(in.Id).Int64s()).Data(g.Map{cols.DeletedAt: gtime.Now(), cols.DeletedBy: contexts.GetUserId(ctx)}).Update()
	if err != nil {
		return gerror.Wrap(err, "删除上架通知公告失败")
	}
	return nil
}

func (s *sSysPublish) NoticeMaxSort(ctx context.Context, in *sysin.NoticeMaxSortInp) (res *adminin.NoticeMaxSortModel, err error) {
	res = new(adminin.NoticeMaxSortModel)
	cols := pdao.YoubanPublishNotice.Columns()
	value, err := pdao.YoubanPublishNotice.Ctx(ctx).
		WhereNull(cols.DeletedAt).
		Fields(cols.Sort).
		OrderDesc(cols.Sort).
		Value()
	if err != nil {
		return nil, gerror.Wrap(err, "获取上架通知公告最大排序失败")
	}
	if value != nil {
		res.Sort = value.Int()
	}
	res.Sort = form.DefaultMaxSort(res.Sort)
	return res, nil
}

func (s *sSysPublish) NoticeStatus(ctx context.Context, in *sysin.NoticeStatusInp) (err error) {
	if in.Id <= 0 {
		return gerror.New("通知公告ID不能为空")
	}
	if !validate.InSlice(consts.StatusSlice, in.Status) {
		return gerror.New("通知公告状态不正确")
	}
	cols := pdao.YoubanPublishNotice.Columns()
	_, err = pdao.YoubanPublishNotice.Ctx(ctx).Where(cols.Id, in.Id).WhereNull(cols.DeletedAt).Data(g.Map{cols.Status: in.Status, cols.UpdatedBy: contexts.GetUserId(ctx), cols.UpdatedAt: gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) NoticePullMessages(ctx context.Context, in *sysin.PullMessagesInp) (res *adminin.PullMessagesModel, err error) {
	accountID := contexts.GetUserId(ctx)
	if accountID <= 0 {
		return nil, gerror.New("获取上架账号信息失败")
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.visibleNoticeRows(ctx, accountID, limit)
	if err != nil {
		return nil, err
	}
	res = &adminin.PullMessagesModel{List: make([]*adminin.PullMessagesRow, 0, len(rows)), NoticeUnreadCountModel: &adminin.NoticeUnreadCountModel{}}
	for _, row := range rows {
		item := pullMessagesRow(row)
		item.IsRead, err = s.noticeIsRead(ctx, row.Id, accountID)
		if err != nil {
			return nil, err
		}
		res.List = append(res.List, item)
		if !item.IsRead {
			switch item.Type {
			case consts.NoticeTypeNotify:
				res.NotifyCount++
			case consts.NoticeTypeNotice:
				res.NoticeCount++
			case consts.NoticeTypeLetter:
				res.LetterCount++
			}
		}
	}
	return res, nil
}

func (s *sSysPublish) NoticeUpRead(ctx context.Context, id int64) (err error) {
	accountID := contexts.GetUserId(ctx)
	if accountID <= 0 || id <= 0 {
		return gerror.New("通知公告参数不正确")
	}
	cols := pdao.YoubanPublishNoticeRead.Columns()
	var read *pentity.YoubanPublishNoticeRead
	err = pdao.YoubanPublishNoticeRead.Ctx(ctx).Where(cols.NoticeId, id).Where(cols.AccountId, accountID).Scan(&read)
	if err != nil {
		return gerror.Wrap(err, "获取通知公告阅读记录失败")
	}
	if read == nil || read.Id <= 0 {
		_, err = pdao.YoubanPublishNoticeRead.Ctx(ctx).Data(g.Map{cols.NoticeId: id, cols.AccountId: accountID, cols.Clicks: 1, cols.CreatedAt: gtime.Now(), cols.UpdatedAt: gtime.Now()}).Insert()
		return err
	}
	_, err = pdao.YoubanPublishNoticeRead.Ctx(ctx).Where(cols.Id, read.Id).Data(g.Map{cols.Clicks: read.Clicks + 1, cols.UpdatedAt: gtime.Now()}).Update()
	return err
}

func (s *sSysPublish) NoticeReadAll(ctx context.Context, in *sysin.NoticeReadAllInp) (err error) {
	rows, err := s.visibleNoticeRows(ctx, contexts.GetUserId(ctx), 0)
	if err != nil {
		return err
	}
	for _, row := range rows {
		if row.Type == int(in.Type) {
			if err = s.NoticeUpRead(ctx, row.Id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *sSysPublish) NoticeMessageList(ctx context.Context, in *sysin.NoticeMessageListInp) (list []*adminin.NoticeMessageListModel, totalCount int, err error) {
	rows, err := s.visibleNoticeRows(ctx, contexts.GetUserId(ctx), 0)
	if err != nil {
		return nil, 0, err
	}
	filtered := make([]*pentity.YoubanPublishNotice, 0, len(rows))
	for _, row := range rows {
		if row.Type == int(in.Type) {
			filtered = append(filtered, row)
		}
	}
	totalCount = len(filtered)
	start := (in.Page - 1) * in.PerPage
	if start < 0 {
		start = 0
	}
	if start >= len(filtered) {
		return []*adminin.NoticeMessageListModel{}, totalCount, nil
	}
	end := start + in.PerPage
	if in.PerPage <= 0 || end > len(filtered) {
		end = len(filtered)
	}
	accountID := contexts.GetUserId(ctx)
	for _, row := range filtered[start:end] {
		read, readErr := s.noticeIsRead(ctx, row.Id, accountID)
		if readErr != nil {
			return nil, 0, readErr
		}
		list = append(list, &adminin.NoticeMessageListModel{Id: row.Id, Type: int64(row.Type), Title: row.Title, Content: row.Content, Tag: row.Tag, Sort: row.Sort, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt, IsRead: read})
	}
	return list, totalCount, nil
}

func (s *sSysPublish) validateNoticeReceivers(ctx context.Context, receiverIDs []int64) error {
	account, err := s.currentAdminAccount(ctx)
	if err != nil {
		return err
	}
	cols := pdao.YoubanPublishAccount.Columns()
	count, err := pdao.YoubanPublishAccount.Ctx(ctx).Where(cols.TenantId, account.TenantId).Wheref("(%s = ? OR %s = ?)", cols.Id, cols.ParentId, account.Id, account.Id).WhereIn(cols.Id, receiverIDs).Where(cols.Status, consts.StatusEnabled).WhereNull(cols.DeletedAt).Count()
	if err != nil {
		return gerror.Wrap(err, "校验通知公告接收人失败")
	}
	if count != len(receiverIDs) {
		return gerror.New("接收人不合法")
	}
	return nil
}

func (s *sSysPublish) noticeReceiverGroup(ctx context.Context, receiverIDs []int64) ([]form.AvatarGroup, error) {
	if len(receiverIDs) == 0 {
		return []form.AvatarGroup{}, nil
	}
	cols := pdao.YoubanPublishAccount.Columns()
	rows := make([]form.AvatarGroup, 0, len(receiverIDs))
	err := pdao.YoubanPublishAccount.Ctx(ctx).Fields(cols.Nickname+" AS name", cols.AvatarUrl+" AS src").WhereIn(cols.Id, receiverIDs).WhereNull(cols.DeletedAt).Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "获取通知公告接收人失败")
	}
	return rows, nil
}

func (s *sSysPublish) visibleNoticeRows(ctx context.Context, accountID int64, limit int) ([]*pentity.YoubanPublishNotice, error) {
	if accountID <= 0 {
		return nil, gerror.New("获取上架账号信息失败")
	}
	cols := pdao.YoubanPublishNotice.Columns()
	mod := pdao.YoubanPublishNotice.Ctx(ctx).WhereNull(cols.DeletedAt).Where(cols.Status, consts.StatusEnabled).Where("(publish_at IS NULL OR publish_at<=?)", gtime.Now()).Where("(expire_at IS NULL OR expire_at>?)", gtime.Now()).OrderDesc(cols.Sort).OrderDesc(cols.Id)
	if limit > 0 {
		mod = mod.Limit(limit)
	}
	var rows []*pentity.YoubanPublishNotice
	if err := mod.Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取上架通知公告失败")
	}
	visible := make([]*pentity.YoubanPublishNotice, 0, len(rows))
	for _, row := range rows {
		if row.Type != consts.NoticeTypeLetter || containsID(parseReceiver(row.Receiver), accountID) {
			visible = append(visible, row)
		}
	}
	return visible, nil
}

func (s *sSysPublish) noticeIsRead(ctx context.Context, noticeID, accountID int64) (bool, error) {
	count, err := pdao.YoubanPublishNoticeRead.Ctx(ctx).Where(pdao.YoubanPublishNoticeRead.Columns().NoticeId, noticeID).Where(pdao.YoubanPublishNoticeRead.Columns().AccountId, accountID).Count()
	return count > 0, err
}

func noticeEntity(row *pentity.YoubanPublishNotice) entity.AdminNotice {
	return entity.AdminNotice{Id: row.Id, Type: int64(row.Type), Title: row.Title, Content: row.Content, Tag: int(row.Tag), Receiver: gjson.New(row.Receiver), Remark: row.Remark, Sort: int(row.Sort), Status: row.Status, PublishAt: row.PublishAt, ExpireAt: row.ExpireAt, CreatedBy: row.CreatedBy, UpdatedBy: row.UpdatedBy, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt}
}

func noticeListModel(row *pentity.YoubanPublishNotice) *adminin.NoticeListModel {
	return &adminin.NoticeListModel{AdminNotice: noticeEntity(row), ReceiverGroup: []form.AvatarGroup{}}
}

func pullMessagesRow(row *pentity.YoubanPublishNotice) *adminin.PullMessagesRow {
	return &adminin.PullMessagesRow{Id: row.Id, Type: int64(row.Type), Title: row.Title, Content: row.Content, Tag: row.Tag, Sort: row.Sort, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt}
}

func parseReceiver(value string) []int64 {
	if strings.TrimSpace(value) == "" || value == "null" {
		return []int64{}
	}
	return g.NewVar(gjson.New(value).Array()).Int64s()
}

func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func noticeShouldPushNow(in *sysin.NoticeEditInp) bool {
	now := gtime.Now()
	return in.Status == consts.StatusEnabled && (in.PublishAt == nil || !in.PublishAt.After(now)) && (in.ExpireAt == nil || in.ExpireAt.After(now))
}
