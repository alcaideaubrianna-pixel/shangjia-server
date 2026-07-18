// Package sysin
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sysin

import (
	"encoding/json"
	"github.com/gogf/gf/v2/frame/g"
	"hotgo/addons/lazysheep_tggo/model"
)

type GetConfigInp struct {
	Group string `json:"group"`
}

type GetConfigModel struct {
	List g.Map `json:"list"`
}

type UpdateConfigInp struct {
	Group string `json:"group"`
	List  g.Map  `json:"list"`
}

func (in *GetConfigInp) ToModel(state *model.State) *GetConfigModel {
	return &GetConfigModel{List: g.Map{"state": state}}
}

func (in *UpdateConfigInp) ToState() *model.State {
	state := model.NewState()
	if raw, ok := in.List["state"]; ok {
		switch v := raw.(type) {
		case string:
			_ = json.Unmarshal([]byte(v), state)
		default:
			data, _ := json.Marshal(v)
			_ = json.Unmarshal(data, state)
		}
	}
	state.Normalize()
	return state
}

type BotUpsertInp struct {
	Key           string `json:"key"`
	Role          string `json:"role"`
	MemberId      int64  `json:"memberId"`
	Token         string `json:"token"`
	DisplayName   string `json:"displayName"`
	Username      string `json:"username"`
	WebhookSecret string `json:"webhookSecret"`
	WebhookPath   string `json:"webhookPath"`
	Enabled       bool   `json:"enabled"`
	AutoPull      bool   `json:"autoPull"`
	AutoForward   bool   `json:"autoForward"`
	ReviewEnabled bool   `json:"reviewEnabled"`
}

type BotInspectInp struct {
	Token string `json:"token"`
}

type BotInspectModel struct {
	Id          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	IsBot       bool   `json:"isBot"`
}

type BotDeleteInp struct {
	Key string `json:"key"`
}

type BotStartInp struct {
	Key string `json:"key"`
}

type TelegramProxyTestInp struct {
	TelegramProxy string `json:"telegramProxy"`
}

type TelegramProxyTestModel struct {
	Ok bool `json:"ok"`
}

type PullMonitorInp struct {
	BotKey  string `json:"botKey"`
	StartAt string `json:"startAt"`
	EndAt   string `json:"endAt"`
	Section string `json:"section"`
}

type PullMonitorModel struct {
	Summary  PullMonitorSummary           `json:"summary"`
	Bindings []*PullMonitorBindingSummary `json:"bindings"`
	Buckets  []*PullMonitorBucket         `json:"buckets"`
	Recent   []*PullMonitorEvent          `json:"recent"`
}

type PullMonitorSummary struct {
	Total        int   `json:"total"`
	Success      int   `json:"success"`
	Failed       int   `json:"failed"`
	AvgElapsedMs int64 `json:"avgElapsedMs"`
}

type PullMonitorBindingSummary struct {
	BotKey             string `json:"botKey"`
	BotName            string `json:"botName"`
	BindingKey         string `json:"bindingKey"`
	SourceURL          string `json:"sourceUrl"`
	ChatID             int64  `json:"chatId"`
	ChatTitle          string `json:"chatTitle"`
	ChatLabel          string `json:"chatLabel"`
	AutoPull           bool   `json:"autoPull"`
	AutoPullStoppedAt  string `json:"autoPullStoppedAt"`
	AutoPullStopReason string `json:"autoPullStopReason"`
	Total              int    `json:"total"`
	Success            int    `json:"success"`
	Failed             int    `json:"failed"`
	Fetched            int    `json:"fetched"`
	Stored             int    `json:"stored"`
	Pushed             int    `json:"pushed"`
	FailedCount        int    `json:"failedCount"`
	PushFailed         int    `json:"pushFailed"`
	AvgElapsedMs       int64  `json:"avgElapsedMs"`
	LastStatus         bool   `json:"lastStatus"`
	LastError          string `json:"lastError"`
	LastAt             string `json:"lastAt"`
}

type PullMonitorBucket struct {
	Time         string                 `json:"time"`
	TimeUnix     int64                  `json:"timeUnix"`
	Total        int                    `json:"total"`
	Success      int                    `json:"success"`
	Failed       int                    `json:"failed"`
	AvgElapsedMs int64                  `json:"avgElapsedMs"`
	Fetched      int                    `json:"fetched"`
	Stored       int                    `json:"stored"`
	Pushed       int                    `json:"pushed"`
	PushFailed   int                    `json:"pushFailed"`
	Steps        []*PullMonitorStepStat `json:"steps"`
}

type PullMonitorEvent struct {
	TraceID       string            `json:"traceId"`
	BotKey        string            `json:"botKey"`
	BotName       string            `json:"botName"`
	BindingKey    string            `json:"bindingKey"`
	SourceURL     string            `json:"sourceUrl"`
	ChatID        int64             `json:"chatId"`
	ChatTitle     string            `json:"chatTitle"`
	ChatLabel     string            `json:"chatLabel"`
	Auto          bool              `json:"auto"`
	Success       bool              `json:"success"`
	Error         string            `json:"error"`
	Message       string            `json:"message"`
	Fetched       int               `json:"fetched"`
	Stored        int               `json:"stored"`
	Pushed        int               `json:"pushed"`
	Deduped       int               `json:"deduped"`
	Skipped       int               `json:"skipped"`
	FailedCount   int               `json:"failedCount"`
	PushFailed    int               `json:"pushFailed"`
	ElapsedMs     int64             `json:"elapsedMs"`
	Steps         []PullMonitorStep `json:"steps"`
	CreatedAt     string            `json:"createdAt"`
	CreatedAtUnix int64             `json:"createdAtUnix"`
	VisibleAtUnix int64             `json:"visibleAtUnix"`
}

type PushQueueMonitorInp struct {
	BotKey string `json:"botKey"`
	ChatID int64  `json:"chatId"`
	Limit  int    `json:"limit"`
}

type PushQueueMonitorModel struct {
	Paused     bool                     `json:"paused"`
	Summary    []*PushQueueStatusCount  `json:"summary"`
	Channels   []*PushQueueChannelModel `json:"channels"`
	Recent     []*PushQueueTaskModel    `json:"recent"`
	FailedLogs []*PushQueueLogModel     `json:"failedLogs"`
}

type PushQueueStatusCount struct {
	Status int    `json:"status"`
	Label  string `json:"label"`
	Count  int    `json:"count"`
}

type PushQueueChannelModel struct {
	BotKey     string `json:"botKey"`
	BotName    string `json:"botName"`
	BindingKey string `json:"bindingKey"`
	ChatID     int64  `json:"chatId"`
	ChatTitle  string `json:"chatTitle"`
	ChatLabel  string `json:"chatLabel"`
	Ready      int    `json:"ready"`
	Doing      int    `json:"doing"`
	Retry      int    `json:"retry"`
	Done       int    `json:"done"`
	Dead       int    `json:"dead"`
	Unknown    int    `json:"unknown"`
	Backlog    int    `json:"backlog"`
	LastError  string `json:"lastError"`
	OldestAt   string `json:"oldestAt"`
}

type PushQueueTaskModel struct {
	Id          int64  `json:"id"`
	BotKey      string `json:"botKey"`
	BotName     string `json:"botName"`
	BindingKey  string `json:"bindingKey"`
	SourceURL   string `json:"sourceUrl"`
	NoteID      int64  `json:"noteId"`
	ContentID   int64  `json:"contentId"`
	ChatID      int64  `json:"chatId"`
	ChatTitle   string `json:"chatTitle"`
	ChatLabel   string `json:"chatLabel"`
	Status      int    `json:"status"`
	StatusLabel string `json:"statusLabel"`
	Attempts    int    `json:"attempts"`
	MaxAttempts int    `json:"maxAttempts"`
	LastError   string `json:"lastError"`
	CreatedAt   string `json:"createdAt"`
	StartedAt   string `json:"startedAt"`
	FinishedAt  string `json:"finishedAt"`
	NextRetryAt string `json:"nextRetryAt"`
}

type PushQueueLogModel struct {
	Id         int64  `json:"id"`
	TaskID     int64  `json:"taskId"`
	BotKey     string `json:"botKey"`
	BotName    string `json:"botName"`
	BindingKey string `json:"bindingKey"`
	NoteID     int64  `json:"noteId"`
	ContentID  int64  `json:"contentId"`
	ChatID     int64  `json:"chatId"`
	ChatTitle  string `json:"chatTitle"`
	ChatLabel  string `json:"chatLabel"`
	Status     int    `json:"status"`
	Attempt    int    `json:"attempt"`
	ElapsedMs  int64  `json:"elapsedMs"`
	Error      string `json:"error"`
	CreatedAt  string `json:"createdAt"`
}

type PushQueueControlInp struct {
	Paused bool   `json:"paused"`
	Action string `json:"action"`
}

type BindingAutoPullControlInp struct {
	BindingKey string `json:"bindingKey"`
	AutoPull   bool   `json:"autoPull"`
}

type ChannelListInp struct {
	BotKey  string `json:"botKey"`
	Keyword string `json:"keyword"`
	Status  string `json:"status"`
}

type ChannelListModel struct {
	List []*ChannelListItem `json:"list"`
}

type ChannelListItem struct {
	BotKey             string `json:"botKey"`
	BotName            string `json:"botName"`
	BindingKey         string `json:"bindingKey"`
	SourceURL          string `json:"sourceUrl"`
	ChatID             int64  `json:"chatId"`
	ChatTitle          string `json:"chatTitle"`
	ChatUsername       string `json:"chatUsername"`
	ChatLabel          string `json:"chatLabel"`
	ChatType           string `json:"chatType"`
	AddedBy            int64  `json:"addedBy"`
	AddedAt            string `json:"addedAt"`
	UpdatedAt          string `json:"updatedAt"`
	LastPullID         int64  `json:"lastPullId"`
	LastCursor         string `json:"lastCursor"`
	AutoPull           bool   `json:"autoPull"`
	AutoPullStoppedAt  string `json:"autoPullStoppedAt"`
	AutoPullStopReason string `json:"autoPullStopReason"`
	BindingStatus      string `json:"bindingStatus"`
	WorkStatus         string `json:"workStatus"`
	WorkStatusType     string `json:"workStatusType"`
	NoteCount          int    `json:"noteCount"`
	Pending            int    `json:"pending"`
	Doing              int    `json:"doing"`
	Retry              int    `json:"retry"`
	Done               int    `json:"done"`
	Dead               int    `json:"dead"`
	Unknown            int    `json:"unknown"`
	LastError          string `json:"lastError"`
}

type PullMonitorStep struct {
	Name      string `json:"name"`
	StepMs    int64  `json:"stepMs"`
	ElapsedMs int64  `json:"elapsedMs"`
}

type PullMonitorStepStat struct {
	Name  string `json:"name"`
	AvgMs int64  `json:"avgMs"`
	Count int    `json:"count"`
}

type TouchUserInp struct {
	TelegramID   int64  `json:"telegramId"`
	BotKey       string `json:"botKey"`
	Username     string `json:"username"`
	FirstName    string `json:"firstName"`
	LastName     string `json:"lastName"`
	LanguageCode string `json:"languageCode"`
	IsBot        bool   `json:"isBot"`
}

type BotUserListInp struct {
	BotKey      string `json:"botKey"`
	Keyword     string `json:"keyword"`
	MemberLevel int    `json:"memberLevel"`
	Status      int    `json:"status"`
}

type BotUserListModel struct {
	Id             int     `json:"id"`
	TelegramID     int64   `json:"telegramId"`
	BotKey         string  `json:"botKey"`
	Username       string  `json:"username"`
	FirstName      string  `json:"firstName"`
	LastName       string  `json:"lastName"`
	LanguageCode   string  `json:"languageCode"`
	IsBot          bool    `json:"isBot"`
	MemberLevel    int     `json:"memberLevel"`
	Points         float64 `json:"points"`
	MemberExpireAt string  `json:"memberExpireAt"`
	Status         int     `json:"status"`
	LastActiveAt   string  `json:"lastActiveAt"`
	CreatedAt      string  `json:"createdAt"`
}

type BotUserEditInp struct {
	Id             int     `json:"id"`
	MemberLevel    int     `json:"memberLevel"`
	Points         float64 `json:"points"`
	MemberExpireAt string  `json:"memberExpireAt"`
	Status         int     `json:"status"`
}

type BindSourceInp struct {
	BotKey        string `json:"botKey"`
	SourceURL     string `json:"sourceUrl"`
	SourceToken   string `json:"sourceToken"`
	ReviewChatID  int64  `json:"reviewChatId"`
	PublishChatID int64  `json:"publishChatId"`
	ChatID        int64  `json:"chatId"`
	OperatorID    int64  `json:"operatorId"`
	Mode          string `json:"mode"`
	AutoPush      bool   `json:"autoPush"`
}

type PullInp struct {
	BotKey    string `json:"botKey"`
	SourceURL string `json:"sourceUrl"`
	ChatID    int64  `json:"chatId"`
	Limit     int    `json:"limit"`
	Auto      bool   `json:"auto"`
	Retry     bool   `json:"retry"`
	Sync      bool   `json:"sync"`
}

type AutoPullTask struct {
	BotKey     string `json:"botKey"`
	BindingKey string `json:"bindingKey"`
	SourceURL  string `json:"sourceUrl"`
	ChatID     int64  `json:"chatId"`
	Slot       int    `json:"slot"`
	QueuedAt   string `json:"queuedAt"`
}

type PushNoteTask struct {
	TaskID     int64  `json:"taskId"`
	BotKey     string `json:"botKey"`
	BindingKey string `json:"bindingKey"`
	SourceURL  string `json:"sourceUrl"`
	NoteID     int64  `json:"noteId"`
	ContentID  int64  `json:"contentId"`
	ChatID     int64  `json:"chatId"`
	Attempt    int    `json:"attempt"`
	QueuedAt   string `json:"queuedAt"`
}

type SignInInp struct {
	BotKey string `json:"botKey"`
	UserID int64  `json:"userId"`
}
