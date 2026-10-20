package consts

import (
	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

var ProfileCollectMethodOptions = []*model.Option{
	dict.GenInfoOption(sysin.CollectSourceTypeAccount, "协议号采集"),
	dict.GenInfoOption(sysin.CollectSourceTypeBot, "Bot采集"),
	dict.GenInfoOption(sysin.CollectSourceTypeFollow, "关注采集"),
	dict.GenDefaultOption("unknown", "历史方式未知"),
}

func init() {
	dict.RegisterEnums("YoubanPublishCollectMethodOptions", "资料采集方式", ProfileCollectMethodOptions)
}
