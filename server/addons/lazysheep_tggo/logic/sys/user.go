// Package sys
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sys

import (
	"context"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"hotgo/addons/lazysheep_tggo/model/input/sysin"
	"hotgo/internal/dao"
)

func (s *sLazySheepTGGo) botUsers(ctx context.Context, in *sysin.BotUserListInp) ([]*sysin.BotUserListModel, error) {
	if in == nil || strings.TrimSpace(in.BotKey) == "" {
		return []*sysin.BotUserListModel{}, nil
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	mod := dao.AddonLazysheepTggoUser.Ctx(ctx).Where("bot_key", strings.TrimSpace(in.BotKey))
	if in.Keyword != "" {
		keyword := "%" + strings.TrimSpace(in.Keyword) + "%"
		mod = mod.WhereLike(cols.Username, keyword).WhereOrLike(cols.FirstName, keyword).WhereOrLike(cols.LastName, keyword)
	}
	if in.MemberLevel > 0 {
		mod = mod.Where(cols.MemberLevel, in.MemberLevel)
	}
	if in.Status > 0 {
		mod = mod.Where(cols.Status, in.Status)
	}
	var rows []struct {
		Id             int
		TelegramId     int64
		BotKey         string
		Username       string
		FirstName      string
		LastName       string
		LanguageCode   string
		IsBot          int
		MemberLevel    int
		Points         float64
		MemberExpireAt *gtime.Time
		Status         int
		LastActiveAt   *gtime.Time
		CreatedAt      *gtime.Time
	}
	if err := mod.Fields(cols.Id, cols.TelegramId, cols.BotKey, cols.Username, cols.FirstName, cols.LastName, cols.LanguageCode, cols.IsBot, cols.MemberLevel, cols.Points, "member_expire_at", cols.Status, cols.LastActiveAt, cols.CreatedAt).
		OrderDesc(cols.LastActiveAt).
		OrderDesc(cols.Id).
		Scan(&rows); err != nil {
		return nil, gerror.Wrap(err, "获取机器人用户失败")
	}
	list := make([]*sysin.BotUserListModel, 0, len(rows))
	for _, row := range rows {
		list = append(list, &sysin.BotUserListModel{
			Id:             row.Id,
			TelegramID:     row.TelegramId,
			BotKey:         row.BotKey,
			Username:       row.Username,
			FirstName:      row.FirstName,
			LastName:       row.LastName,
			LanguageCode:   row.LanguageCode,
			IsBot:          row.IsBot > 0,
			MemberLevel:    row.MemberLevel,
			Points:         row.Points,
			MemberExpireAt: timeString(row.MemberExpireAt),
			Status:         row.Status,
			LastActiveAt:   timeString(row.LastActiveAt),
			CreatedAt:      timeString(row.CreatedAt),
		})
	}
	return list, nil
}

func (s *sLazySheepTGGo) updateBotUser(ctx context.Context, in *sysin.BotUserEditInp) error {
	if in == nil || in.Id <= 0 {
		return gerror.New("用户ID不能为空")
	}
	cols := dao.AddonLazysheepTggoUser.Columns()
	expireAt, err := parseBotUserExpireAt(in.MemberExpireAt)
	if err != nil {
		return err
	}
	_, err = dao.AddonLazysheepTggoUser.Ctx(ctx).
		Where(cols.Id, in.Id).
		Data(g.Map{
			cols.MemberLevel:   in.MemberLevel,
			cols.Points:        in.Points,
			"member_expire_at": expireAt,
			cols.Status:        in.Status,
			cols.UpdatedAt:     gtime.Now(),
		}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "更新机器人用户失败")
	}
	return nil
}

func parseBotUserExpireAt(raw string) (*gtime.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
	}
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, raw, time.Local)
		if err == nil {
			return gtime.NewFromTime(t), nil
		}
	}
	return nil, gerror.New("会员到期时间格式不正确")
}

func timeString(t *gtime.Time) string {
	if t == nil {
		return ""
	}
	return t.String()
}
