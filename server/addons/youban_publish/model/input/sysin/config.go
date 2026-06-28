package sysin

import (
	"github.com/gogf/gf/v2/frame/g"

	basesysin "hotgo/internal/model/input/sysin"
)

type GetConfigInp struct {
	basesysin.GetAddonsConfigInp
}

type GetConfigModel struct {
	List g.Map `json:"list"`
}

type UpdateConfigInp struct {
	basesysin.UpdateAddonsConfigInp
}
