package streamgray

import "github.com/gogf/gf/v2/errors/gerror"

// ErrInvalidRuntime 表示灰度 Stream 运行器依赖未注入完整。
var ErrInvalidRuntime = gerror.New("Redis Stream runtime 参数无效")
