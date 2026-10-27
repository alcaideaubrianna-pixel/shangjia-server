package sys

import (
	"context"
	publishsysin "hotgo/addons/youban_publish/model/input/sysin"
	"regexp"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_bot/model/input/sysin"
	"hotgo/addons/youban_bot/service"
	gatewayservice "hotgo/addons/youban_tg_bot_gateway/service"
)

const (
	botTable                = "hg_youban_bot_bot"
	featureTable            = "hg_youban_bot_feature"
	authCodeTable           = "hg_youban_bot_auth_code"
	accountBindTbl          = "hg_youban_bot_account_bind"
	userTable               = "hg_youban_bot_user"
	messageTable            = "hg_youban_bot_message"
	channelCacheTable       = "hg_youban_bot_channel_cache"
	customEmojiTable        = "hg_youban_bot_custom_emoji"
	broadcastTaskTable      = "hg_youban_bot_broadcast_task"
	broadcastRecipientTable = "hg_youban_bot_broadcast_recipient"
	broadcastTaskBotTable   = "hg_youban_bot_broadcast_task_bot"

	publishAccountTable = "hg_youban_publish_account"
)

var sixDigitRegexp = regexp.MustCompile(`\b\d{6}\b`)

type sSysBot struct {
	managerMu             sync.Mutex
	managerCancel         context.CancelFunc
	managerDone           chan struct{}
	relayClientForToken   func(context.Context, string) (*tgbot.Bot, error)
	relayAccountForUser   func(context.Context, int64) (*botProfileAccount, error)
	relayOwnedBots        func(context.Context, int64, int64, int64, bool) ([]*publishsysin.RelayOwnedBotModel, error)
	relayMu               sync.Mutex
	relayCancel           context.CancelFunc
	relayDone             chan struct{}
	cleanupMu             sync.Mutex
	cleanupCancel         context.CancelFunc
	featureMu             sync.RWMutex
	features              map[string]*botFeatureRow
	featureAt             time.Time
	featureDefaultsAt     time.Time
	broadcastMu           sync.Mutex
	broadcastRunning      bool
	profileRecoveryMu     sync.Mutex
	profileRecoveryCancel context.CancelFunc
}

type authCodeRow struct {
	Id               int64       `json:"id"`
	Code             string      `json:"code"`
	Scene            string      `json:"scene"`
	App              string      `json:"app"`
	AccountId        int64       `json:"account_id"`
	TelegramUserId   string      `json:"telegram_user_id"`
	TelegramUsername string      `json:"telegram_username"`
	LoginToken       string      `json:"login_token"`
	Status           string      `json:"status"`
	ErrorMessage     string      `json:"error_message"`
	ExpiresAt        *gtime.Time `json:"expires_at"`
}

func (row *authCodeRow) statusModel() *sysin.CodeStatusModel {
	if row == nil {
		return nil
	}
	return &sysin.CodeStatusModel{
		AccountId:        row.AccountId,
		App:              row.App,
		Code:             row.Code,
		ErrorMessage:     row.ErrorMessage,
		ExpiresAt:        row.ExpiresAt,
		Scene:            row.Scene,
		Status:           row.Status,
		TelegramUserId:   row.TelegramUserId,
		TelegramUsername: row.TelegramUsername,
		Token:            row.LoginToken,
	}
}

func init() {
	bot := NewSysBot()
	registerBotManagerAccountHandler(bot)
	service.RegisterSysBot(bot)
	gatewayservice.RegisterProvider(&botGatewayProvider{bot: bot})
	gatewayservice.RegisterFeature(&relayGatewayFeature{bot: bot})
}

func NewSysBot() *sSysBot { return &sSysBot{} }

func (s *sSysBot) StartRuntime(ctx context.Context) {
	s.startBotManagerRuntime(ctx)
	_ = s.syncAllTelegramBotMenus(ctx)
	s.startTelegramMessageCleanup(ctx)
	s.startPendingBroadcasts(ctx)
	s.startProfileSessionRecovery(ctx)
	s.startRelayRuntime(ctx)
}
func (s *sSysBot) StopRuntime() {
	s.stopBotManagerRuntime()
	s.stopTelegramMessageCleanup()
	s.stopProfileSessionRecovery()
	s.stopRelayRuntime()
}

type botFeature interface {
	Key() string
	Command() string
	Description() string
	Handle(ctx context.Context, bot *sSysBot, featureCtx *botFeatureContext) (handled bool, err error)
}

type botFeatureConfigProvider interface {
	ConfigSchema() []*sysin.FeatureConfigSchema
}

type botFeatureTextMatcher interface {
	Match(ctx context.Context, bot *sSysBot, row *botFeatureRow, text string) bool
}

type botFeatureContext struct {
	BotId int64
	Msg   *models.Message
	Text  string
	Args  string
}

type botFeatureRow struct {
	Id          int64       `json:"id"`
	FeatureKey  string      `json:"feature_key"`
	Name        string      `json:"name"`
	Command     string      `json:"command"`
	Description string      `json:"description"`
	ConfigJson  string      `json:"config_json"`
	Sort        int         `json:"sort"`
	Status      int         `json:"status"`
	CreatedAt   *gtime.Time `json:"created_at"`
	UpdatedAt   *gtime.Time `json:"updated_at"`
}

var botFeatures []botFeature

func registerBotFeature(feature botFeature) {
	if feature == nil || strings.TrimSpace(feature.Key()) == "" {
		return
	}
	botFeatures = append(botFeatures, feature)
}

func init() {
	registerBotFeature(relayFeature{})
	registerBotFeature(startFeature{})
	registerBotFeature(helpFeature{})
	registerBotFeature(relayManagementFeature{})
	registerBotFeature(botManagerFeature{})
	registerBotFeature(inlinePromotionFeature{})
	registerBotFeature(loginFeature{})
	registerBotFeature(bindFeature{})
	registerBotFeature(infoFeature{})
	registerBotFeature(notifyFeature{})
	registerBotFeature(superNotifyFeature{})
	registerBotFeature(contactFeature{})
	registerBotFeature(adminFeature{})
	registerBotFeature(inviteFeature{})
	registerBotFeature(accountManagementFeature{})
	registerBotFeature(instantRegisterFeature{})
	registerBotFeature(profileFeature{})
	registerBotFeature(profilePublishFeature{})
	registerBotFeature(collectManageFeature{})
	registerBotFeature(scanFeature{})
	registerBotFeature(quickPushFeature{})
	registerBotFeature(exchangeRateFeature{})
}
