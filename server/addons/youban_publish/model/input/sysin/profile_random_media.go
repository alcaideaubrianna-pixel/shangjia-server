package sysin

import "github.com/gogf/gf/v2/errors/gerror"

// Enabled=nil follows the image-count default; omitted RandomMedia preserves
// an existing configuration when older clients save a profile.
type ProfileRandomMediaPolicy struct {
	Enabled *bool `json:"enabled"`
	Count   int   `json:"count"`
}

func (in *ProfileRandomMediaPolicy) Filter() error {
	if in.Count == 0 {
		in.Count = 5
	}
	if in.Count < 1 {
		return gerror.New("随机展示图片数量必须大于0")
	}
	return nil
}
