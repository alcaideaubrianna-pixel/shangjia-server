package chat

import (
	"hotgo/addons/youban_chat/model/input/sysin"
	"hotgo/internal/model/input/form"

	"github.com/gogf/gf/v2/frame/g"
)

type ListReq struct {
	g.Meta `path:"/chat/list" method:"get" tags:"悦伴聊天后台" summary:"客服会话列表"`
	sysin.AdminChatConversationListInp
}

type ListRes struct {
	form.PageRes
	List []*sysin.AdminChatConversationListModel `json:"list" dc:"会话列表"`
}

type ViewReq struct {
	g.Meta `path:"/chat/view" method:"get" tags:"悦伴聊天后台" summary:"客服会话详情"`
	sysin.AdminChatConversationViewInp
}

type ViewRes struct {
	*sysin.AdminChatConversationViewModel
}

type MessagesReq struct {
	g.Meta `path:"/chat/messages" method:"get" tags:"悦伴聊天后台" summary:"客服聊天记录"`
	sysin.AdminChatMessageListInp
}

type MessagesRes struct {
	form.PageRes
	List []*sysin.ChatMessageModel `json:"list" dc:"消息列表"`
}

type BotListReq struct {
	g.Meta `path:"/chat/botList" method:"get" tags:"悦伴聊天后台" summary:"Bot列表"`
	sysin.AdminChatBotListInp
}

type BotListRes struct {
	form.PageRes
	List []*sysin.AdminChatBotModel `json:"list" dc:"Bot列表"`
}

type SaveBotReq struct {
	g.Meta `path:"/chat/saveBot" method:"post" tags:"悦伴聊天后台" summary:"保存Bot"`
	sysin.AdminChatBotSaveInp
}

type SaveBotRes struct{}

type BindingListReq struct {
	g.Meta `path:"/chat/bindingList" method:"get" tags:"悦伴聊天后台" summary:"频道绑定列表"`
	sysin.AdminChatBindingListInp
}

type BindingListRes struct {
	form.PageRes
	List []*sysin.AdminChatBindingModel `json:"list" dc:"频道绑定列表"`
}

type SaveBindingReq struct {
	g.Meta `path:"/chat/saveBinding" method:"post" tags:"悦伴聊天后台" summary:"保存频道绑定"`
	sysin.AdminChatBindingSaveInp
}

type SaveBindingRes struct{}

type ChannelOptionsReq struct {
	g.Meta `path:"/chat/channelOptions" method:"get" tags:"悦伴聊天后台" summary:"频道选项"`
}

type ChannelOptionsRes struct {
	List []*sysin.AdminChatChannelOptionModel `json:"list" dc:"频道选项"`
}

type OperatorListReq struct {
	g.Meta `path:"/chat/operatorList" method:"get" tags:"悦伴聊天后台" summary:"客服绑定列表"`
	sysin.AdminChatOperatorListInp
}

type OperatorListRes struct {
	form.PageRes
	List []*sysin.AdminChatOperatorModel `json:"list" dc:"客服列表"`
}

type SaveOperatorReq struct {
	g.Meta `path:"/chat/saveOperator" method:"post" tags:"悦伴聊天后台" summary:"保存客服绑定"`
	sysin.AdminChatOperatorSaveInp
}

type SaveOperatorRes struct{}

type FeatureListReq struct {
	g.Meta `path:"/chat/featureList" method:"get" tags:"悦伴聊天后台" summary:"Telegram功能列表"`
	sysin.AdminChatFeatureListInp
}

type FeatureListRes struct {
	form.PageRes
	List []*sysin.AdminChatFeatureModel `json:"list" dc:"功能列表"`
}

type SaveFeatureReq struct {
	g.Meta `path:"/chat/saveFeature" method:"post" tags:"悦伴聊天后台" summary:"保存Telegram功能配置"`
	sysin.AdminChatFeatureSaveInp
}

type SaveFeatureRes struct{}
