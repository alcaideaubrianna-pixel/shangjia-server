package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/global"
	"hotgo/internal/dao"
)

func (s *sSysConfig) ensureAntiScanConfigRows(ctx context.Context) error {
	rows := []g.Map{
		{"name": "背景纹理预设", "type": "string", "key": "backgroundTexturePreset", "value": "rabbit", "default_value": "rabbit", "sort": 445, "tip": "背景纹理预设 rabbit/heart/dot/grid"},
		{"name": "素材库背景贴图", "type": "string", "key": "backgroundTextureImage", "value": "", "default_value": "", "sort": 446, "tip": "素材库背景贴图地址，留空使用预设"},
		{"name": "手动遮挡素材", "type": "string", "key": "maskItemsJson", "value": "[]", "default_value": "[]", "sort": 495, "tip": "手动摆放二维码或贴图 JSON"},
	}
	for _, row := range rows {
		key := gconv.String(row["key"])
		count, err := dao.SysAddonsConfig.Ctx(ctx).
			Where("addon_name", global.GetSkeleton().Name).
			Where("group", publishConfigGroupAntiScan).
			Where("key", key).
			Count()
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		fillAntiScanConfigRow(row)
		if _, err = dao.SysAddonsConfig.Ctx(ctx).Data(row).Insert(); err != nil {
			return err
		}
	}
	return nil
}

func fillAntiScanConfigRow(row g.Map) {
	now := gtime.Now()
	row["addon_name"] = global.GetSkeleton().Name
	row["group"] = publishConfigGroupAntiScan
	row["is_default"] = 1
	row["status"] = 1
	row["created_at"] = now
	row["updated_at"] = now
}
