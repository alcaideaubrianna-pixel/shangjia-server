// Package sysin
// @Link  https://github.com/bufanyun/hotgo
// @Copyright  Copyright (c) 2023 HotGo CLI
// @Author  Codex
// @License  https://github.com/bufanyun/hotgo/blob/master/LICENSE
package sysin

type NoteStoreInp struct {
	BotKey     string `json:"botKey"`
	BindingKey string `json:"bindingKey"`
	Payload    string `json:"payload"`
}

type NoteStoreModel struct {
	NoteId int64  `json:"noteId"`
	Code   string `json:"code"`
}
