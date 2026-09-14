package sys

import (
	"context"

	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
	"github.com/gogf/gf/v2/util/gconv"

	"hotgo/addons/youban_publish/global"
	"hotgo/internal/dao"
)

func (s *sSysConfig) ensureCloudResourceConfigRows(ctx context.Context) error {
	rows := []g.Map{
		{"name": "人像抠图来源", "type": "string", "key": "mattingProvider", "value": "facepp", "default_value": "facepp", "sort": 40, "tip": "aliyun/tencent/fapihub/facepp"},
		{"name": "阿里云 AccessKey ID", "type": "string", "key": "aliyunAccessKeyId", "value": "", "default_value": "", "sort": 50, "tip": "阿里云视觉智能开放平台 AccessKey ID"},
		{"name": "阿里云 AccessKey Secret", "type": "string", "key": "aliyunAccessKeySecret", "value": "", "default_value": "", "sort": 60, "tip": "页面回显会脱敏"},
		{"name": "阿里云 Endpoint", "type": "string", "key": "aliyunEndpoint", "value": "imageseg.cn-shanghai.aliyuncs.com", "default_value": "imageseg.cn-shanghai.aliyuncs.com", "sort": 70, "tip": "SegmentBody 上海地域 Endpoint"},
		{"name": "腾讯抠图处理桶", "type": "string", "key": "tencentMattingBucket", "value": "", "default_value": "", "sort": 80, "tip": "A 账号已开通数据万象的桶，格式 BucketName-APPID"},
		{"name": "腾讯抠图临时目录", "type": "string", "key": "tencentMattingPath", "value": "youban-matting", "default_value": "youban-matting", "sort": 90, "tip": "A 账号处理桶内的临时对象目录"},
		{"name": "Face++ API Key", "type": "string", "key": "facePlusApiKey", "value": "", "default_value": "", "sort": 100, "tip": "Face++ 人体抠像 API Key"},
		{"name": "Face++ API Secret", "type": "string", "key": "facePlusApiSecret", "value": "", "default_value": "", "sort": 110, "tip": "页面回显会脱敏"},
		{"name": "Face++ Endpoint", "type": "string", "key": "facePlusEndpoint", "value": "https://api-cn.faceplusplus.com/humanbodypp/v2/segment", "default_value": "https://api-cn.faceplusplus.com/humanbodypp/v2/segment", "sort": 120, "tip": "Face++ 人体抠像接口地址"},
		{"name": "Face++ 并发数", "type": "number", "key": "facePlusConcurrency", "value": "2", "default_value": "2", "sort": 130, "tip": "Face++ API 并发限制"},
	}
	for _, row := range rows {
		key := gconv.String(row["key"])
		count, err := dao.SysAddonsConfig.Ctx(ctx).
			Where("addon_name", global.GetSkeleton().Name).
			Where("group", publishConfigGroupCloudResource).
			Where("key", key).
			Count()
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		fillCloudResourceConfigRow(row)
		if _, err = dao.SysAddonsConfig.Ctx(ctx).Data(row).Insert(); err != nil {
			return err
		}
	}
	return nil
}

func fillCloudResourceConfigRow(row g.Map) {
	now := gtime.Now()
	row["addon_name"] = global.GetSkeleton().Name
	row["group"] = publishConfigGroupCloudResource
	row["is_default"] = 1
	row["status"] = 1
	row["created_at"] = now
	row["updated_at"] = now
}
