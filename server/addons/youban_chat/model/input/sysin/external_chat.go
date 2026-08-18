package sysin

type ExternalVisitorInp struct {
	AppId          string `json:"-"`
	ExternalUserId string `json:"externalUserId" v:"required|length:1,128#用户标识不能为空|用户标识过长"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	AvatarUrl      string `json:"avatarUrl"`
}

type ExternalSessionInp struct {
	Visitor   ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ProfileId int64              `json:"profileId"`
}

type ExternalMessageInp struct {
	Visitor          ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ConversationId   int64              `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	Content          string             `json:"content"`
	ClientMessageId  string             `json:"clientMessageId"`
	ReplyToMessageId int64              `json:"replyToMessageId"`
}

type ExternalMessagesInp struct {
	Visitor        ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ConversationId int64              `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	AfterId        int64              `json:"afterId"`
}

type ExternalFileInp struct {
	ExternalMessageInp
	FileName      string `json:"fileName" v:"required#文件名不能为空"`
	MimeType      string `json:"mimeType"`
	ContentBase64 string `json:"contentBase64" v:"required#文件内容不能为空"`
}

type ExternalReadInp struct {
	Visitor        ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ConversationId int64              `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	LastMessageId  int64              `json:"lastMessageId"`
}

type ExternalUnreadInp struct {
	Visitor ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
}

type ExternalConversationsInp struct {
	Visitor ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
}
type ExternalConversationModel struct {
	Id             int64  `json:"id"`
	ConversationId int64  `json:"conversationId"`
	ProfileId      int64  `json:"profileId"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	UnreadCount    int    `json:"unreadCount"`
	LastMessage    string `json:"lastMessage"`
	LastMessageAt  string `json:"lastMessageAt"`
	Avatar         string `json:"avatar"`
	IsPinned       bool   `json:"isPinned"`
	CanDelete      bool   `json:"canDelete"`
}
type ExternalConversationsModel struct {
	List []*ExternalConversationModel `json:"list"`
}

type ExternalConversationActionInp struct {
	Visitor        ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ConversationId int64              `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	Pinned         bool               `json:"pinned"`
}

type ExternalAdminListInp struct {
	AppId   string `json:"-"`
	Page    int    `json:"page"`
	PerPage int    `json:"pageSize"`
	Keyword string `json:"keyword"`
}
type ExternalAdminBotSaveInp struct {
	AppId    string `json:"-"`
	BotToken string `json:"botToken" v:"required#Bot Token不能为空"`
	Remark   string `json:"remark"`
}
type ExternalAdminBotModel struct {
	Id          int64  `json:"id"`
	BotName     string `json:"botName"`
	BotUsername string `json:"botUsername"`
	TokenHint   string `json:"tokenHint"`
	Remark      string `json:"remark"`
	Status      int    `json:"status"`
	BindingId   int64  `json:"bindingId"`
	BindCode    string `json:"bindCode"`
	TgChatId    string `json:"tgChatId"`
	TgChatTitle string `json:"tgChatTitle"`
	IsBound     bool   `json:"isBound"`
}
type ExternalAdminBotListModel struct {
	List  []*ExternalAdminBotModel `json:"list"`
	Total int                      `json:"total"`
}
type ExternalAdminBotActionInp struct {
	AppId string `json:"-"`
	Id    int64  `json:"id" v:"required|min:1#Bot ID不能为空|Bot ID不能为空"`
}
type ExternalAdminConversationInp struct {
	AppId          string `json:"-"`
	ConversationId int64  `json:"conversationId"`
	Page           int    `json:"page"`
	PerPage        int    `json:"pageSize"`
}
type ExternalAdminConversationListModel struct {
	List  []*ExternalConversationModel `json:"list"`
	Total int                          `json:"total"`
}

type ExternalReactionInp struct {
	Visitor        ExternalVisitorInp `json:"visitor" v:"required#访客信息不能为空"`
	ConversationId int64              `json:"conversationId" v:"required|min:1#会话ID不能为空|会话ID不能为空"`
	MessageId      int64              `json:"messageId" v:"required|min:1#消息ID不能为空|消息ID不能为空"`
	Emoji          string             `json:"emoji" v:"required#表情不能为空"`
	Remove         bool               `json:"remove"`
}
