package consts

import (
	"hotgo/internal/library/dict"
	"hotgo/internal/model"
)

const (
	PublishBotTypeNormal = "normal"
	PublishBotTypeTwoWay = "two_way"
)

var PublishBotTypeOptions = []*model.Option{
	dict.GenDefaultOption(PublishBotTypeNormal, "普通机器人"),
	dict.GenInfoOption(PublishBotTypeTwoWay, "双向机器人"),
}

func init() {
	dict.RegisterEnums("YoubanPublishBotTypeOptions", "后台Bot用途", PublishBotTypeOptions)
}
