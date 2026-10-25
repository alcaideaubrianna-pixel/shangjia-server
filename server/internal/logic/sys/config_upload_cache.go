package sys

import (
	"context"
	"time"

	"hotgo/internal/consts"
	"hotgo/internal/dao"
	"hotgo/internal/library/cache"
	"hotgo/internal/model/entity"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
)

const uploadConfigCacheTTL = 30 * time.Minute

func (s *sSysConfig) getUploadConfigValues(ctx context.Context) (g.Map, error) {
	if cache.Initialized() {
		cached, err := cache.Instance().Get(ctx, consts.CacheSysConfigUpload)
		if err == nil && !cached.IsNil() {
			values := make(g.Map)
			if scanErr := cached.Scan(&values); scanErr == nil && len(values) > 0 {
				return values, nil
			}
		}
		if err != nil {
			g.Log().Warningf(ctx, "读取上传配置缓存失败，已降级读取数据库: %+v", err)
		}
	}

	values, err := s.getConfigValuesFromDB(ctx, "upload")
	if err != nil {
		return nil, err
	}
	if cache.Initialized() {
		if err = cache.Instance().Set(ctx, consts.CacheSysConfigUpload, values, uploadConfigCacheTTL); err != nil {
			g.Log().Warningf(ctx, "回填上传配置缓存失败: %+v", err)
		}
	}
	return values, nil
}

func (s *sSysConfig) refreshUploadConfigCache(ctx context.Context) error {
	values, err := s.getConfigValuesFromDB(ctx, "upload")
	if err != nil {
		return err
	}
	if !cache.Initialized() {
		return nil
	}
	// 先删除旧值；即使写入失败，后续读取也会安全降级到数据库。
	if _, err = cache.Instance().Remove(ctx, consts.CacheSysConfigUpload); err != nil {
		g.Log().Warningf(ctx, "清理旧上传配置缓存失败: %+v", err)
	}
	if err = cache.Instance().Set(ctx, consts.CacheSysConfigUpload, values, uploadConfigCacheTTL); err != nil {
		return gerror.Wrap(err, "刷新上传配置缓存失败")
	}
	return nil
}

func (s *sSysConfig) getConfigValuesFromDB(ctx context.Context, group string) (g.Map, error) {
	var rows []*entity.SysConfig
	cols := dao.SysConfig.Columns()
	if err := dao.SysConfig.Ctx(ctx).
		Fields(cols.Key, cols.Value, cols.Type).
		Where(cols.Group, group).
		Scan(&rows); err != nil {
		return nil, gerror.Wrapf(err, "获取配置分组[ %v ]失败，请稍后重试！", group)
	}

	if len(rows) == 0 {
		return nil, nil
	}
	values := make(g.Map, len(rows))
	for _, row := range rows {
		value, err := s.ConversionType(ctx, row)
		if err != nil {
			return nil, err
		}
		values[row.Key] = value
	}
	return values, nil
}

func cloneConfigValues(values g.Map) g.Map {
	if values == nil {
		return nil
	}
	copy := make(g.Map, len(values))
	for key, value := range values {
		copy[key] = value
	}
	return copy
}
