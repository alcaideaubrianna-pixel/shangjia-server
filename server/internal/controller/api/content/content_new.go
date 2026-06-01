package content

import apicontent "hotgo/api/api/content"

type ControllerV1 struct{}

func NewV1() apicontent.IContentV1 {
	return &ControllerV1{}
}
