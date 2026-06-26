package sysin

import (
	"hotgo/internal/model/input/form"
)

type ChatStartInp struct {
	ProfileId int64 `json:"profileId" dc:"资料ID，0表示全局客服"`
}

type ChatStartModel struct {
	Id             int64  `json:"id"             dc:"本地会话ID"`
	ConversationId int64  `json:"conversationId" dc:"会话ID"`
	ContactId      int64  `json:"contactId"      dc:"兼容字段"`
	ProfileId      int64  `json:"profileId"      dc:"资料ID"`
	Status         string `json:"status"         dc:"状态"`
}

type ChatSendInp struct {
	ConversationId  int64  `json:"conversationId"  v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	Content         string `json:"content"         v:"required#消息不能为空"                   dc:"消息内容"`
	ClientMessageId string `json:"clientMessageId" dc:"客户端消息ID"`
}

type ChatSendModel struct {
	MessageId       int64  `json:"messageId"       dc:"消息ID"`
	ClientMessageId string `json:"clientMessageId" dc:"客户端消息ID"`
	Status          string `json:"status"          dc:"状态"`
}

type ChatMessagesInp struct {
	ConversationId int64 `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	AfterId        int64 `json:"afterId"        dc:"只返回此消息ID之后的数据"`
}

type ChatConversationPinInp struct {
	ConversationId int64 `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	Pinned         int   `json:"pinned"         dc:"是否置顶：1置顶 0取消"`
}

type ChatConversationClearInp struct {
	ConversationId int64 `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
}

type ChatMessageAttachmentModel struct {
	Id          int64  `json:"id"          dc:"附件ID"`
	Name        string `json:"name"        dc:"文件名"`
	FileType    string `json:"fileType"    dc:"附件类型"`
	DataUrl     string `json:"dataUrl"     dc:"资源地址"`
	ThumbUrl    string `json:"thumbUrl"    dc:"缩略图地址"`
	FallbackUrl string `json:"fallbackUrl" dc:"兜底地址"`
	Data        []byte `json:"-"           dc:"临时文件内容"`
}

type ChatMessageModel struct {
	Id              int64                         `json:"id"             dc:"消息ID"`
	ClientMessageId string                        `json:"clientMessageId" dc:"客户端消息ID"`
	ConversationId  int64                         `json:"conversationId" dc:"会话ID"`
	Direction       string                        `json:"direction"      dc:"方向：mine/service/system"`
	Content         string                        `json:"content"        dc:"消息内容"`
	ContentType     string                        `json:"contentType"    dc:"内容类型"`
	Status          string                        `json:"status"         dc:"状态"`
	SenderName      string                        `json:"senderName"     dc:"发送人"`
	CreatedAt       string                        `json:"createdAt"      dc:"创建时间"`
	ReadAt          string                        `json:"readAt"         dc:"已读时间"`
	Attachments     []*ChatMessageAttachmentModel `json:"attachments"    dc:"附件"`
}

type ChatMessagesModel struct {
	List []*ChatMessageModel `json:"list" dc:"消息列表"`
}

type ChatReadInp struct {
	ConversationId int64 `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	LastMessageId  int64 `json:"lastMessageId"  dc:"最后已读消息ID"`
}

type ChatUploadInp struct {
	ConversationId int64  `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	Content        string `json:"content"        dc:"附件说明"`
}

type ChatUploadModel struct {
	Message *ChatMessageModel `json:"message" dc:"消息"`
}

type ChatUnreadModel struct {
	UnreadCount int `json:"unreadCount" dc:"未读数"`
}

type TelegramWebhookInp struct {
	UpdateId      int64               `json:"update_id" dc:"Telegram更新ID"`
	BotId         int64               `json:"botId"     dc:"Bot ID"`
	Message       *TelegramMessageInp `json:"message"   dc:"消息"`
	EditedMessage *TelegramMessageInp `json:"edited_message" dc:"编辑消息"`
}

type TelegramMessageInp struct {
	MessageId       int64                `json:"message_id"`
	MessageThreadId int64                `json:"message_thread_id"`
	Text            string               `json:"text"`
	Caption         string               `json:"caption"`
	Chat            *TelegramChatInp     `json:"chat"`
	From            *TelegramUserInp     `json:"from"`
	ReplyTo         *TelegramMessageInp  `json:"reply_to_message"`
	Entities        []*TelegramEntityInp `json:"entities"`
	CaptionEntities []*TelegramEntityInp `json:"caption_entities"`
	Photo           []*TelegramPhotoInp  `json:"photo"`
	Video           *TelegramFileInp     `json:"video"`
	Document        *TelegramFileInp     `json:"document"`
	Sticker         *TelegramStickerInp  `json:"sticker"`
	Animation       *TelegramFileInp     `json:"animation"`
}

type TelegramPhotoInp struct {
	FileId       string `json:"file_id"`
	FileUniqueId string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	FileSize     int64  `json:"file_size"`
}

type TelegramFileInp struct {
	FileId       string `json:"file_id"`
	FileUniqueId string `json:"file_unique_id"`
	FileName     string `json:"file_name"`
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"`
}

type TelegramStickerInp struct {
	FileId       string            `json:"file_id"`
	FileUniqueId string            `json:"file_unique_id"`
	Type         string            `json:"type"`
	Emoji        string            `json:"emoji"`
	SetName      string            `json:"set_name"`
	FileSize     int64             `json:"file_size"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	IsAnimated   bool              `json:"is_animated"`
	IsVideo      bool              `json:"is_video"`
	Thumbnail    *TelegramPhotoInp `json:"thumbnail"`
}

type TelegramChatInp struct {
	Id       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type TelegramUserInp struct {
	Id        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}

type TelegramEntityInp struct {
	Type          string `json:"type"`
	Offset        int    `json:"offset"`
	Length        int    `json:"length"`
	CustomEmojiId string `json:"custom_emoji_id"`
}

type ChatConversationListInp struct {
	form.PageReq
}

type ChatConversationListModel struct {
	Id             int64  `json:"id"             dc:"本地会话ID"`
	ConversationId int64  `json:"conversationId" dc:"会话ID"`
	ProfileId      int64  `json:"profileId"      dc:"资料ID"`
	IsGlobal       bool   `json:"isGlobal"       dc:"是否全局客服"`
	IsPinned       bool   `json:"isPinned"       dc:"是否置顶"`
	CanDelete      bool   `json:"canDelete"      dc:"是否允许移除会话入口"`
	ProfileNo      string `json:"profileNo"      dc:"资料编号"`
	Name           string `json:"name"           dc:"显示名称"`
	Avatar         string `json:"avatar"         dc:"头像"`
	Province       string `json:"province"       dc:"省份"`
	City           string `json:"city"           dc:"城市"`
	Age            int    `json:"age"            dc:"年龄"`
	Height         int    `json:"height"         dc:"身高"`
	LastMessage    string `json:"lastMessage"    dc:"最后消息"`
	LastMessageAt  string `json:"lastMessageAt"  dc:"最后消息时间"`
	UnreadCount    int    `json:"unreadCount"    dc:"未读数"`
	Status         string `json:"status"         dc:"状态"`
}

type ChatWidgetSessionInp struct {
	ProfileId int64 `json:"profileId" dc:"资料ID"`
}

type ChatWidgetUserModel struct {
	Identifier   string `json:"identifier"   dc:"用户唯一标识"`
	Name         string `json:"name"         dc:"显示名称"`
	Email        string `json:"email"        dc:"邮箱"`
	Phone        string `json:"phone"        dc:"手机号"`
	AvatarUrl    string `json:"avatarUrl"    dc:"头像"`
	IdentityHash string `json:"identityHash" dc:"身份签名"`
}

type ChatWidgetProfileModel struct {
	Id        int64  `json:"id"        dc:"资料ID"`
	ProfileNo string `json:"profileNo" dc:"资料编号"`
	Title     string `json:"title"     dc:"标题"`
	Province  string `json:"province"  dc:"省份"`
	City      string `json:"city"      dc:"城市"`
	Age       int    `json:"age"       dc:"年龄"`
	Height    int    `json:"height"    dc:"身高"`
}

type ChatWidgetSessionModel struct {
	WebsiteToken     string                  `json:"websiteToken"     dc:"兼容字段"`
	LauncherTitle    string                  `json:"launcherTitle"    dc:"启动文案"`
	User             *ChatWidgetUserModel    `json:"user"             dc:"用户身份"`
	Profile          *ChatWidgetProfileModel `json:"profile"          dc:"资料上下文"`
	CustomAttributes map[string]interface{}  `json:"customAttributes" dc:"自定义属性"`
}

type AdminChatConversationListInp struct {
	form.PageReq
	Keyword    string `json:"keyword"    dc:"关键词"`
	MemberId   int64  `json:"memberId"   dc:"会员ID"`
	ProfileId  int64  `json:"profileId"  dc:"资料ID"`
	Status     string `json:"status"     dc:"会话状态"`
	HasTgTopic int    `json:"hasTgTopic" dc:"是否有关联TG话题：1是 2否"`
}

type AdminChatConversationListModel struct {
	Id                int64  `json:"id"                  dc:"会话ID"`
	MemberId          int64  `json:"memberId"            dc:"会员ID"`
	MemberUsername    string `json:"memberUsername"      dc:"会员账号"`
	MemberRealName    string `json:"memberRealName"      dc:"会员姓名"`
	MemberMobile      string `json:"memberMobile"        dc:"会员手机号"`
	MemberEmail       string `json:"memberEmail"         dc:"会员邮箱"`
	ProfileId         int64  `json:"profileId"           dc:"资料ID"`
	ProfileNo         string `json:"profileNo"           dc:"资料编号"`
	ProfileTitle      string `json:"profileTitle"        dc:"资料标题"`
	Province          string `json:"province"            dc:"省份"`
	City              string `json:"city"                dc:"城市"`
	ChatSessionId     string `json:"chatSessionId" dc:"会话标识"`
	TgChatId          string `json:"tgChatId"            dc:"TG群ID"`
	TgMessageThreadId int64  `json:"tgMessageThreadId"   dc:"TG话题ID"`
	LastMessage       string `json:"lastMessage"         dc:"最后消息"`
	LastMessageAt     string `json:"lastMessageAt"       dc:"最后消息时间"`
	UnreadCount       int    `json:"unreadCount"         dc:"未读数"`
	MessageCount      int    `json:"messageCount"        dc:"消息数"`
	Status            string `json:"status"              dc:"状态"`
	CreatedAt         string `json:"createdAt"           dc:"创建时间"`
	UpdatedAt         string `json:"updatedAt"           dc:"更新时间"`
}

type AdminChatConversationViewInp struct {
	Id int64 `json:"id" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
}

type AdminChatConversationClearInp struct {
	ConversationId int64 `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
}

type AdminChatConversationViewModel struct {
	*AdminChatConversationListModel
	MemberAvatar string `json:"memberAvatar" dc:"会员头像"`
	MemberRemark string `json:"memberRemark" dc:"会员备注"`
	ProfileText  string `json:"profileText"  dc:"资料正文"`
}

type AdminChatMessageListInp struct {
	form.PageReq
	ConversationId int64  `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空" dc:"会话ID"`
	Direction      string `json:"direction"      dc:"消息方向"`
}

type AdminChatBotListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status"  dc:"状态"`
}

type AdminChatBotModel struct {
	Id          int64  `json:"id"          dc:"ID"`
	BotName     string `json:"botName"     dc:"Bot名称"`
	BotUsername string `json:"botUsername" dc:"Bot用户名"`
	BotToken    string `json:"botToken"    dc:"Bot Token"`
	Remark      string `json:"remark"      dc:"备注"`
	Status      int    `json:"status"      dc:"状态"`
	CreatedAt   string `json:"createdAt"   dc:"创建时间"`
	UpdatedAt   string `json:"updatedAt"   dc:"更新时间"`
}

type AdminChatBotSaveInp struct {
	Id          int64  `json:"id"          dc:"ID"`
	BotName     string `json:"botName"     dc:"Bot名称"`
	BotUsername string `json:"botUsername" dc:"Bot用户名"`
	BotToken    string `json:"botToken"    dc:"Bot Token"`
	Remark      string `json:"remark"      dc:"备注"`
	Status      int    `json:"status"      dc:"状态"`
}

type AdminChatBindingListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status"  dc:"状态"`
}

type AdminChatBindingModel struct {
	Id               int64   `json:"id"               dc:"ID"`
	BindCode         string  `json:"bindCode"         dc:"绑定码"`
	BindType         string  `json:"bindType"         dc:"绑定类型：global/channel"`
	SourceChannelId  int64   `json:"sourceChannelId"  dc:"来源频道ID"`
	ContentChannelId int64   `json:"contentChannelId" dc:"本地频道ID"`
	ChannelIds       []int64 `json:"channelIds"       dc:"关联频道ID"`
	ChannelTitle     string  `json:"channelTitle"     dc:"频道标题"`
	ChannelUsername  string  `json:"channelUsername"  dc:"频道用户名"`
	BotId            int64   `json:"botId"            dc:"Bot ID"`
	BotName          string  `json:"botName"          dc:"Bot名称"`
	TgChatId         string  `json:"tgChatId"         dc:"TG群ID"`
	TgChatTitle      string  `json:"tgChatTitle"      dc:"TG群标题"`
	Remark           string  `json:"remark"           dc:"备注"`
	Status           int     `json:"status"           dc:"状态"`
	CreatedAt        string  `json:"createdAt"        dc:"创建时间"`
	UpdatedAt        string  `json:"updatedAt"        dc:"更新时间"`
}

type AdminChatBindingSaveInp struct {
	Id               int64   `json:"id"               dc:"ID"`
	BindCode         string  `json:"bindCode"         dc:"绑定码"`
	BindType         string  `json:"bindType"         dc:"绑定类型：global/channel"`
	SourceChannelId  int64   `json:"sourceChannelId"  dc:"来源频道ID"`
	ContentChannelId int64   `json:"contentChannelId" dc:"本地频道ID"`
	ChannelIds       []int64 `json:"channelIds"       dc:"关联频道ID"`
	BotId            int64   `json:"botId"            dc:"Bot ID"`
	TgChatId         string  `json:"tgChatId"         dc:"TG群ID"`
	TgChatTitle      string  `json:"tgChatTitle"      dc:"TG群标题"`
	Remark           string  `json:"remark"           dc:"备注"`
	Status           int     `json:"status"           dc:"状态"`
}

type AdminChatChannelOptionModel struct {
	Label string `json:"label" dc:"选项名称"`
	Value int64  `json:"value" dc:"频道ID"`
}

type AdminChatOperatorListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status"  dc:"状态"`
}

type AdminChatOperatorModel struct {
	Id               int64  `json:"id"               dc:"ID"`
	AdminMemberId    int64  `json:"adminMemberId"    dc:"后台会员ID"`
	AdminUsername    string `json:"adminUsername"    dc:"后台账号"`
	AdminRealName    string `json:"adminRealName"    dc:"后台姓名"`
	TelegramUserId   string `json:"telegramUserId"   dc:"TG用户ID"`
	TelegramUsername string `json:"telegramUsername" dc:"TG用户名"`
	DisplayName      string `json:"displayName"      dc:"显示名称"`
	BindCode         string `json:"bindCode"         dc:"绑定码"`
	Remark           string `json:"remark"           dc:"备注"`
	Status           int    `json:"status"           dc:"状态"`
	CreatedAt        string `json:"createdAt"        dc:"创建时间"`
	UpdatedAt        string `json:"updatedAt"        dc:"更新时间"`
}

type AdminChatOperatorSaveInp struct {
	Id               int64  `json:"id"               dc:"ID"`
	AdminMemberId    int64  `json:"adminMemberId"    dc:"后台会员ID"`
	TelegramUserId   string `json:"telegramUserId"   dc:"TG用户ID"`
	TelegramUsername string `json:"telegramUsername" dc:"TG用户名"`
	DisplayName      string `json:"displayName"      dc:"显示名称"`
	BindCode         string `json:"bindCode"         dc:"绑定码"`
	Remark           string `json:"remark"           dc:"备注"`
	Status           int    `json:"status"           dc:"状态"`
}

type AdminChatFeatureListInp struct {
	form.PageReq
	Keyword string `json:"keyword" dc:"关键词"`
	Status  int    `json:"status"  dc:"状态"`
}

type AdminChatFeatureModel struct {
	Id          int64  `json:"id"          dc:"ID"`
	FeatureKey  string `json:"featureKey"  dc:"功能Key"`
	Name        string `json:"name"        dc:"功能名称"`
	Command     string `json:"command"     dc:"Telegram命令"`
	Description string `json:"description" dc:"功能描述"`
	ConfigJson  string `json:"configJson"  dc:"配置JSON"`
	Sort        int    `json:"sort"        dc:"排序"`
	Status      int    `json:"status"      dc:"状态"`
	CreatedAt   string `json:"createdAt"   dc:"创建时间"`
	UpdatedAt   string `json:"updatedAt"   dc:"更新时间"`
}

type AdminChatFeatureSaveInp struct {
	Id          int64  `json:"id"          dc:"ID"`
	FeatureKey  string `json:"featureKey"  dc:"功能Key"`
	Name        string `json:"name"        dc:"功能名称"`
	Command     string `json:"command"     dc:"Telegram命令"`
	Description string `json:"description" dc:"功能描述"`
	ConfigJson  string `json:"configJson"  dc:"配置JSON"`
	Sort        int    `json:"sort"        dc:"排序"`
	Status      int    `json:"status"      dc:"状态"`
}
