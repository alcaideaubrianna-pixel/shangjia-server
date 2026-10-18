// Package dbtime 统一解释旧业务表的无时区时间。
//
// 强制约定：timestamp without time zone / MySQL datetime 中保存的是
// Asia/Shanghai 钟面时间。读取后必须先调用 FromTimestamp 或 FromTime，
// 再与当前时间比较、计算截止时间或生成 Unix 时间戳。禁止在业务层复制
// time.Date 重建时区的代码，禁止通过 time.Local 猜测数据库时间语义。
//
// 本包不能用于 timestamptz、RFC3339、Telegram Unix 时间、缓存 Unix 时间戳
// 或 time.Now()：这些值已表示绝对时间，只能用 In(Location()) 转换显示时区。
package dbtime

import (
	"time"
	_ "time/tzdata" // 精简容器也必须使用完整 Asia/Shanghai 时区规则。

	"github.com/gogf/gf/v2/os/gtime"
)

var location = func() *time.Location {
	value, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		panic("无法加载数据库业务时区 Asia/Shanghai: " + err.Error())
	}
	return value
}()

// Location 返回无时区业务表唯一使用的时区，与进程 TZ、time.Local 无关。
func Location() *time.Location { return location }

// FromTime 将已确认来自无时区数据库字段的钟面值解释为业务时间。
// 必须保留年月日时分秒而不是调用 In：驱动可能错误地给钟面值附带 UTC。
// 零值保持不变；转换幂等，不修改输入，也不访问数据库或缓存。
func FromTime(value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	return time.Date(value.Year(), value.Month(), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond(), location)
}

// FromTimestamp 是 GoFrame 无时区数据库字段的统一读取入口。
// nil/零值保持原语义；返回新值，禁止修改共享的缓存对象。
func FromTimestamp(value *gtime.Time) *gtime.Time {
	if value == nil {
		return nil
	}
	return gtime.NewFromTime(FromTime(value.Time))
}
