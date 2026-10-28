package sys

import (
	"context"
	"strings"
	"unicode"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/youban_bot/internal/dao"
	"hotgo/addons/youban_bot/internal/model/do"
	"hotgo/addons/youban_bot/model/input/sysin"
)

func normalizeFeatureCommand(command string) string {
	return strings.ToLower(strings.TrimLeftFunc(strings.TrimSpace(command), func(r rune) bool {
		return r == '/' || unicode.IsSpace(r)
	}))
}

// Use the same fallback as dispatch; clearing a configured command does not
// disable the built-in command. /start is reserved regardless of legacy rows.
func effectiveFeatureCommand(feature botFeature, row *botFeatureRow) string {
	if feature.Key() == (startFeature{}).Key() {
		return "start"
	}
	if feature.Key() == (relayFeature{}).Key() {
		return ""
	}
	if row != nil && strings.TrimSpace(row.Command) != "" {
		return normalizeFeatureCommand(row.Command)
	}
	return normalizeFeatureCommand(feature.Command())
}

func validateFeatureCommand(in *sysin.FeatureSaveInp, configs map[string]*botFeatureRow) error {
	in.FeatureKey = strings.TrimSpace(in.FeatureKey)
	command := normalizeFeatureCommand(in.Command)
	var current botFeature
	for _, feature := range botFeatures {
		if feature.Key() == in.FeatureKey {
			current = feature
			break
		}
	}
	if current == nil {
		return gerror.New("插件不存在")
	}
	if in.FeatureKey == "start" {
		if command != "start" || (in.Status != 0 && in.Status != 1) {
			return gerror.New("开始使用是默认插件，命令必须为start且保持启用")
		}
	} else if command == "start" {
		return gerror.New("/start是默认插件保留命令")
	}
	candidate := effectiveFeatureCommand(current, &botFeatureRow{Command: command})
	if candidate != "" {
		for _, feature := range botFeatures {
			if feature.Key() != in.FeatureKey && effectiveFeatureCommand(feature, configs[feature.Key()]) == candidate {
				return gerror.New("该命令已被其他插件使用")
			}
		}
	}
	in.Command = candidate
	if in.FeatureKey == "start" {
		in.Status = 1
	}
	return nil
}

// All plugin metadata saves share this database row lock. Validation reads the
// transaction's current rows, never the process cache, and the lock is held
// through commit so concurrent web instances cannot claim the same command.
func (s *sSysBot) persistFeature(ctx context.Context, in *sysin.FeatureSaveInp) error {
	return dao.YoubanBotFeature.Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		c := dao.YoubanBotFeature.Columns()
		anchor := tx.Model(dao.YoubanBotFeature.Table()).Ctx(ctx).Where(c.FeatureKey, "start").WhereNull(c.DeletedAt)
		sqlite := strings.EqualFold(dao.YoubanBotFeature.DB().GetConfig().Type, "sqlite")
		if sqlite {
			// SQLite has no FOR UPDATE; a write acquires its transaction write lock.
			if _, err := anchor.Data(do.YoubanBotFeature{Command: gdb.Raw(c.Command)}).Update(); err != nil {
				return gerror.Wrap(err, "获取Bot插件配置锁失败")
			}
		} else {
			anchor = anchor.LockUpdate()
		}
		row, err := anchor.One()
		if err != nil {
			return gerror.Wrap(err, "获取Bot插件配置锁失败")
		}
		if len(row) == 0 {
			return gerror.New("默认插件尚未注册，请刷新插件列表")
		}

		var rows []*botFeatureRow
		mod := tx.Model(dao.YoubanBotFeature.Table()).Ctx(ctx).WhereNull(c.DeletedAt).OrderAsc(c.Id)
		if !sqlite {
			mod = mod.LockUpdate()
		} // Current reads also work under MySQL repeatable read.
		if err = mod.Scan(&rows); err != nil {
			return gerror.Wrap(err, "读取Bot插件命令失败")
		}
		configs := make(map[string]*botFeatureRow, len(rows))
		for _, row := range rows {
			configs[row.FeatureKey] = row
		}
		if err = validateFeatureCommand(in, configs); err != nil {
			return err
		}

		status := in.Status
		if status == 0 {
			status = 1
		}
		if in.FeatureKey == (relayFeature{}).Key() {
			return s.persistRelayFeature(ctx, in, g.Map{c.Name: strings.TrimSpace(in.Name), c.Command: "", c.Description: strings.TrimSpace(in.Description), c.ConfigJson: strings.TrimSpace(in.ConfigJson), c.Sort: in.Sort, c.Status: status})
		}
		data := do.YoubanBotFeature{Name: strings.TrimSpace(in.Name), Command: in.Command, Description: strings.TrimSpace(in.Description), ConfigJson: strings.TrimSpace(in.ConfigJson), Sort: in.Sort, Status: status}
		mod = tx.Model(dao.YoubanBotFeature.Table()).Ctx(ctx)
		if in.Id > 0 {
			_, err = mod.Where(c.Id, in.Id).Where(c.FeatureKey, in.FeatureKey).Data(data).Update()
		} else {
			data.FeatureKey = in.FeatureKey
			_, err = mod.Data(data).Insert()
		}
		return err
	})
}
