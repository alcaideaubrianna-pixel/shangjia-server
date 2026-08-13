package sys

import (
	"context"
	"errors"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
)

type broadcastTaskRow struct {
	Id            int64       `json:"id"`
	Text          string      `json:"text"`
	DisableNotice int         `json:"disable_notice"`
	Status        string      `json:"status"`
	TotalCount    int         `json:"total_count"`
	SuccessCount  int         `json:"success_count"`
	FailedCount   int         `json:"failed_count"`
	BlockedCount  int         `json:"blocked_count"`
	LastError     string      `json:"last_error"`
	CreatedAt     *gtime.Time `json:"created_at"`
	StartedAt     *gtime.Time `json:"started_at"`
	FinishedAt    *gtime.Time `json:"finished_at"`
}

type broadcastRecipient struct {
	BotId          int64  `json:"bot_id"`
	ChatId         string `json:"chat_id"`
	TelegramUserId string `json:"telegram_user_id"`
}

func (s *sSysBot) AdminBroadcastCreate(ctx context.Context, in *sysin.BroadcastCreateInp) (*sysin.BroadcastTaskModel, error) {
	if in == nil || strings.TrimSpace(in.Text) == "" {
		return nil, gerror.New("消息内容不能为空")
	}
	if len([]rune(in.Text)) > 4096 {
		return nil, gerror.New("消息内容不能超过4096个字符")
	}
	if err := s.ensureBroadcastTable(ctx); err != nil {
		return nil, err
	}
	botIds := positiveUniqueInt64s(in.BotIds)
	if len(botIds) == 0 {
		if err := g.DB().Model(botTable).Safe().Ctx(ctx).Fields("id").Where("status", 1).Where("is_official", 1).Order("is_default DESC,id ASC").Scan(&botIds); err != nil {
			return nil, gerror.Wrap(err, "读取官方Bot失败")
		}
	}
	var allowedBotIds []int64
	if err := g.DB().Model(botTable).Safe().Ctx(ctx).Fields("id").WhereIn("id", botIds).Where("status", 1).Where("is_official", 1).Order("is_default DESC,id ASC").Scan(&allowedBotIds); err != nil {
		return nil, gerror.Wrap(err, "校验官方Bot失败")
	}
	botIds = allowedBotIds
	if len(botIds) == 0 {
		return nil, gerror.New("没有可用的官方Bot")
	}
	var id int64
	err := g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		result, insertErr := tx.Model(broadcastTaskTable).Ctx(ctx).Data(g.Map{
			"text": strings.TrimSpace(in.Text), "disable_notice": boolInt(in.DisableNotice),
			"status": "pending", "created_at": gtime.Now(), "updated_at": gtime.Now(),
		}).Insert()
		if insertErr != nil {
			return insertErr
		}
		id, insertErr = result.LastInsertId()
		if insertErr != nil {
			return insertErr
		}
		rows := make([]g.Map, 0, len(botIds))
		for _, botId := range botIds {
			rows = append(rows, g.Map{"task_id": id, "bot_id": botId, "created_at": gtime.Now()})
		}
		_, insertErr = tx.Model(broadcastTaskBotTable).Ctx(ctx).Data(rows).Insert()
		return insertErr
	})
	if err != nil {
		return nil, gerror.Wrap(err, "创建全局推送任务失败")
	}
	s.startPendingBroadcasts(ctx)
	return s.AdminBroadcastTask(ctx, &sysin.BroadcastTaskInp{Id: id})
}

func (s *sSysBot) AdminBroadcastTask(ctx context.Context, in *sysin.BroadcastTaskInp) (*sysin.BroadcastTaskModel, error) {
	if in == nil || in.Id <= 0 {
		return nil, gerror.New("任务ID不正确")
	}
	if err := s.ensureBroadcastTable(ctx); err != nil {
		return nil, err
	}
	row := new(broadcastTaskRow)
	if err := g.DB().Model(broadcastTaskTable).Safe().Ctx(ctx).Where("id", in.Id).Scan(row); err != nil {
		return nil, err
	}
	if row.Id == 0 {
		return nil, gerror.New("推送任务不存在")
	}
	botIds, err := s.broadcastTaskBotIds(ctx, row.Id)
	if err != nil {
		return nil, err
	}
	return row.model(botIds), nil
}

func (s *sSysBot) startPendingBroadcasts(ctx context.Context) {
	workerCtx := context.WithoutCancel(ctx)
	s.broadcastMu.Lock()
	if s.broadcastRunning {
		s.broadcastMu.Unlock()
		return
	}
	s.broadcastRunning = true
	s.broadcastMu.Unlock()
	go func() {
		defer func() { s.broadcastMu.Lock(); s.broadcastRunning = false; s.broadcastMu.Unlock() }()
		for {
			row := new(broadcastTaskRow)
			_ = s.ensureBroadcastTable(workerCtx)
			if err := g.DB().Model(broadcastTaskTable).Safe().Ctx(workerCtx).WhereIn("status", []string{"pending", "running"}).Order("id ASC").Limit(1).Scan(row); err != nil || row.Id == 0 {
				return
			}
			s.runBroadcast(workerCtx, row)
		}
	}()
}

func (s *sSysBot) runBroadcast(ctx context.Context, task *broadcastTaskRow) {
	botIds, err := s.broadcastTaskBotIds(ctx, task.Id)
	if err != nil {
		s.finishBroadcast(ctx, task.Id, "failed", 0, 0, 0, 0, err.Error())
		return
	}
	var recipients []*broadcastRecipient
	err = g.DB().Model(userTable+" u").Safe().Ctx(ctx).
		Fields("u.bot_id,u.chat_id,u.telegram_user_id").
		WhereIn("u.bot_id", botIds).Where("u.status", 1).Where("u.chat_type", "private").Where("u.chat_id<>''").
		Where("EXISTS (SELECT 1 FROM " + accountBindTbl + " ab WHERE ab.telegram_user_id=u.telegram_user_id AND ab.status=1 AND ab.deleted_at IS NULL)").
		Order("u.telegram_user_id,u.bot_id").Scan(&recipients)
	if err != nil {
		s.finishBroadcast(ctx, task.Id, "failed", 0, 0, 0, 0, err.Error())
		return
	}
	recipients = uniqueBroadcastRecipients(recipients)
	_, _ = g.DB().Model(broadcastTaskTable).Safe().Ctx(ctx).Where("id", task.Id).Data(g.Map{"status": "running", "total_count": len(recipients), "started_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
	success, failed, blocked := 0, 0, 0
	lastError := ""
	for _, recipient := range recipients {
		row, rowErr := s.botById(ctx, recipient.BotId)
		if rowErr == nil {
			_, rowErr = s.sendMessage(ctx, row.BotToken, recipient.ChatId, task.Text, "HTML", task.DisableNotice == 1)
			var rateErr *tgbot.TooManyRequestsError
			if errors.As(rowErr, &rateErr) && rateErr.RetryAfter > 0 {
				time.Sleep(time.Duration(rateErr.RetryAfter) * time.Second)
				_, rowErr = s.sendMessage(ctx, row.BotToken, recipient.ChatId, task.Text, "HTML", task.DisableNotice == 1)
			}
		}
		if rowErr == nil {
			success++
		} else {
			failed++
			lastError = rowErr.Error()
			if isBroadcastBlocked(rowErr) {
				blocked++
			}
		}
		_, _ = g.DB().Model(broadcastTaskTable).Safe().Ctx(ctx).Where("id", task.Id).Data(g.Map{"success_count": success, "failed_count": failed, "blocked_count": blocked, "last_error": lastError, "updated_at": gtime.Now()}).Update()
		time.Sleep(50 * time.Millisecond)
	}
	s.finishBroadcast(ctx, task.Id, "completed", len(recipients), success, failed, blocked, lastError)
}

func (s *sSysBot) finishBroadcast(ctx context.Context, id int64, status string, total, success, failed, blocked int, lastError string) {
	_, _ = g.DB().Model(broadcastTaskTable).Safe().Ctx(ctx).Where("id", id).Data(g.Map{"status": status, "total_count": total, "success_count": success, "failed_count": failed, "blocked_count": blocked, "last_error": lastError, "finished_at": gtime.Now(), "updated_at": gtime.Now()}).Update()
}

func (s *sSysBot) ensureBroadcastTable(ctx context.Context) error {
	taskSQL := "CREATE TABLE IF NOT EXISTS " + broadcastTaskTable + " (id BIGSERIAL PRIMARY KEY,text TEXT NOT NULL,disable_notice SMALLINT NOT NULL DEFAULT 0,status VARCHAR(32) NOT NULL DEFAULT 'pending',total_count INTEGER NOT NULL DEFAULT 0,success_count INTEGER NOT NULL DEFAULT 0,failed_count INTEGER NOT NULL DEFAULT 0,blocked_count INTEGER NOT NULL DEFAULT 0,last_error TEXT,created_at TIMESTAMP,started_at TIMESTAMP,finished_at TIMESTAMP,updated_at TIMESTAMP)"
	relationSQL := "CREATE TABLE IF NOT EXISTS " + broadcastTaskBotTable + " (id BIGSERIAL PRIMARY KEY,task_id BIGINT NOT NULL,bot_id BIGINT NOT NULL,created_at TIMESTAMP,UNIQUE(task_id,bot_id))"
	if strings.Contains(strings.ToLower(g.DB().GetConfig().Type), "mysql") {
		taskSQL = "CREATE TABLE IF NOT EXISTS " + broadcastTaskTable + " (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,text TEXT NOT NULL,disable_notice TINYINT NOT NULL DEFAULT 0,status VARCHAR(32) NOT NULL DEFAULT 'pending',total_count INT NOT NULL DEFAULT 0,success_count INT NOT NULL DEFAULT 0,failed_count INT NOT NULL DEFAULT 0,blocked_count INT NOT NULL DEFAULT 0,last_error TEXT,created_at DATETIME,started_at DATETIME,finished_at DATETIME,updated_at DATETIME) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
		relationSQL = "CREATE TABLE IF NOT EXISTS " + broadcastTaskBotTable + " (id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,task_id BIGINT NOT NULL,bot_id BIGINT NOT NULL,created_at DATETIME,UNIQUE KEY uk_ybbtb_task_bot(task_id,bot_id),KEY idx_ybbtb_bot(bot_id,task_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
	}
	if _, err := g.DB().Exec(ctx, taskSQL); err != nil {
		return err
	}
	if _, err := g.DB().Exec(ctx, relationSQL); err != nil {
		return err
	}
	dropSQL := "ALTER TABLE " + broadcastTaskTable + " DROP COLUMN IF EXISTS bot_ids_json"
	_, err := g.DB().Exec(ctx, dropSQL)
	return err
}

func (row *broadcastTaskRow) model(botIds []int64) *sysin.BroadcastTaskModel {
	return &sysin.BroadcastTaskModel{Id: row.Id, BotIds: botIds, Text: row.Text, DisableNotice: row.DisableNotice == 1, Status: row.Status, TotalCount: row.TotalCount, SuccessCount: row.SuccessCount, FailedCount: row.FailedCount, BlockedCount: row.BlockedCount, LastError: row.LastError, CreatedAt: row.CreatedAt, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt}
}

func (s *sSysBot) broadcastTaskBotIds(ctx context.Context, taskId int64) ([]int64, error) {
	var botIds []int64
	if err := g.DB().Model(broadcastTaskBotTable).Safe().Ctx(ctx).Fields("bot_id").Where("task_id", taskId).Order("id ASC").Scan(&botIds); err != nil {
		return nil, gerror.Wrap(err, "读取推送Bot失败")
	}
	if len(botIds) == 0 {
		return nil, gerror.New("推送任务没有关联Bot")
	}
	return botIds, nil
}

func positiveUniqueInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func isBroadcastBlocked(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "blocked by the user") || strings.Contains(text, "bot was blocked") || strings.Contains(text, "user is deactivated") || strings.Contains(text, "chat not found")
}

func uniqueBroadcastRecipients(rows []*broadcastRecipient) []*broadcastRecipient {
	seen := make(map[string]struct{}, len(rows))
	result := make([]*broadcastRecipient, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.TelegramUserId == "" || row.ChatId == "" {
			continue
		}
		if _, exists := seen[row.TelegramUserId]; exists {
			continue
		}
		seen[row.TelegramUserId] = struct{}{}
		result = append(result, row)
	}
	return result
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
