package sys

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/library/cache"
)

const profileTagNameCacheKey = "youban_publish:tag:name_map:v2"
const profileTagNameCacheTTL = 10 * time.Minute

func (s *sSysPublish) applyProfileTagNames(ctx context.Context, list []*sysin.ProfileModel) error {
	if len(list) == 0 {
		return nil
	}
	names, err := s.profileTagNameMap(ctx)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.Tag = profileTagText(item.Tag, names)
	}
	return nil
}

func (s *sSysPublish) applyNoteTagNames(ctx context.Context, list []*sysin.NoteModel) error {
	if len(list) == 0 {
		return nil
	}
	names, err := s.profileTagNameMap(ctx)
	if err != nil {
		return err
	}
	for _, item := range list {
		if item == nil {
			continue
		}
		item.Tag = profileTagText(item.Tag, names)
	}
	return nil
}

func (s *sSysPublish) profileTagNameMap(ctx context.Context) (map[string]string, error) {
	var names map[string]string
	cacheVar, err := cache.Instance().Get(ctx, profileTagNameCacheKey)
	if err == nil && !cacheVar.IsNil() {
		if scanErr := cacheVar.Scan(&names); scanErr == nil && names != nil {
			return names, nil
		}
	}
	names, err = s.profileTagNameMapFromDB(ctx)
	if err != nil {
		return nil, err
	}
	_ = cache.Instance().Set(ctx, profileTagNameCacheKey, names, profileTagNameCacheTTL)
	return names, nil
}

func (s *sSysPublish) profileTagNameMapFromDB(ctx context.Context) (map[string]string, error) {
	var rows []struct {
		Id   int64  `orm:"id"`
		Name string `orm:"name"`
	}
	err := g.DB().Model(publishTagTable).Safe().Ctx(ctx).
		Fields("id,name").
		WhereNull("deleted_at").
		Scan(&rows)
	if err != nil {
		return nil, gerror.Wrap(err, "读取标签缓存失败")
	}
	names := make(map[string]string, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(row.Name)
		if row.Id <= 0 || name == "" {
			continue
		}
		names[strconv.FormatInt(row.Id, 10)] = name
		names[name] = name
	}
	return names, nil
}

func (s *sSysPublish) clearProfileTagNameCache(ctx context.Context) {
	_, _ = cache.Instance().Remove(ctx, profileTagNameCacheKey)
}

func profileTagText(value string, names map[string]string) string {
	parts := splitProfileTagValues(value)
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		tag := strings.TrimSpace(part)
		if tag == "" {
			continue
		}
		name, ok := names[tag]
		if !ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return strings.Join(out, ",")
}
