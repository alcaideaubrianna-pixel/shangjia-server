package consts

import (
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

const (
	CollectSourceTypeAccount = "account"
	CollectSourceTypeBot     = "bot"
	CollectSourceTypeFollow  = "follow"
)

var ProfileCollectMethodOptions = []*model.Option{
	dict.GenInfoOption(CollectSourceTypeAccount, "协议号采集"),
	dict.GenInfoOption(CollectSourceTypeBot, "Bot采集"),
	dict.GenInfoOption(CollectSourceTypeFollow, "关注采集"),
	dict.GenDefaultOption("unknown", "历史方式未知"),
}

func init() {
	dict.RegisterEnums("YoubanPublishCollectMethodOptions", "资料采集方式", ProfileCollectMethodOptions)
}
