package sys

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gogf/gf/v2/frame/g"

	"hotgo/internal/dao"
	"hotgo/internal/model/input/sysin"
)

type contentRegionDirectory struct {
	byCode map[string]string
}

type contentRegionDirectoryCacheState struct {
	sync.RWMutex
	value      *contentRegionDirectory
	expiresAt  time.Time
	refreshing bool
}

type contentRegionRow struct {
	Id    int64  `orm:"id"`
	Title string `orm:"title"`
}

const (
	contentRegionDirectoryTTL            = 10 * time.Minute
	contentRegionDirectoryRefreshTimeout = time.Second
)

var contentRegionDirectoryCache contentRegionDirectoryCacheState

func loadContentRegionDirectory(ctx context.Context) *contentRegionDirectory {
	return contentRegionDirectoryCache.load(ctx, queryContentRegionDirectory)
}

func queryContentRegionDirectory(ctx context.Context) (*contentRegionDirectory, error) {
	columns := dao.SysProvinces.Columns()
	rows := make([]contentRegionRow, 0)
	if err := dao.SysProvinces.Ctx(ctx).
		Fields(columns.Id, columns.Title).
		Where(columns.Status, 1).
		Scan(&rows); err != nil {
		return nil, err
	}
	directory := &contentRegionDirectory{
		byCode: make(map[string]string, len(rows)),
	}
	for index := range rows {
		row := rows[index]
		directory.byCode[strconv.FormatInt(row.Id, 10)] = row.Title
	}
	return directory, nil
}

func (c *contentRegionDirectoryCacheState) load(
	ctx context.Context,
	loader func(context.Context) (*contentRegionDirectory, error),
) *contentRegionDirectory {
	now := time.Now()
	c.Lock()
	cached := c.value
	if cached != nil && now.Before(c.expiresAt) {
		c.Unlock()
		return cached
	}
	if c.refreshing {
		c.Unlock()
		return contentRegionDirectoryOrEmpty(cached)
	}
	c.refreshing = true
	c.Unlock()

	refreshCtx := context.WithoutCancel(ctx)
	if cached != nil {
		go c.refresh(refreshCtx, loader)
		return cached
	}
	c.refresh(refreshCtx, loader)

	c.RLock()
	cached = c.value
	c.RUnlock()
	return contentRegionDirectoryOrEmpty(cached)
}

func (c *contentRegionDirectoryCacheState) refresh(
	ctx context.Context,
	loader func(context.Context) (*contentRegionDirectory, error),
) {
	refreshCtx, cancel := context.WithTimeout(ctx, contentRegionDirectoryRefreshTimeout)
	defer cancel()

	directory, err := loader(refreshCtx)
	c.Lock()
	c.refreshing = false
	if err == nil && directory != nil {
		c.value = directory
		c.expiresAt = time.Now().Add(contentRegionDirectoryTTL)
	}
	c.Unlock()
	if err != nil {
		g.Log().Warningf(ctx, "刷新资料地区目录失败，继续使用旧缓存: %+v", err)
	}
}

func contentRegionDirectoryOrEmpty(directory *contentRegionDirectory) *contentRegionDirectory {
	if directory != nil {
		return directory
	}
	return &contentRegionDirectory{byCode: map[string]string{}}
}

func decorateProfileRegionWithDirectory(directory *contentRegionDirectory, profile *sysin.ContentProfileListModel) {
	if profile == nil {
		return
	}
	provinceRaw, cityRaw := strings.TrimSpace(profile.Province), strings.TrimSpace(profile.City)
	province, provinceKnown := resolveRegionCodes(directory, provinceRaw)
	city, cityKnown := resolveRegionCodes(directory, cityRaw)
	if provinceKnown {
		profile.ProvinceCode = provinceRaw
	}
	if cityKnown {
		profile.CityCode = cityRaw
	}
	province, city = normalizeProfileRegionForOption(province, city)
	profile.Province, profile.City = province, city
	profile.LocationLabel = strings.TrimSpace(strings.Join([]string{province, city}, " "))
	if city == "" || city == province {
		profile.LocationLabel = province
	}
}

// resolveRegionCodes handles both a single code and legacy comma-separated codes.
func resolveRegionCodes(directory *contentRegionDirectory, raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' })
	if len(parts) == 0 {
		return cleanRegionToken(raw), false
	}
	labels := make([]string, 0, len(parts))
	known := false
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		label := cleanRegionToken(part)
		if title, ok := directory.byCode[part]; ok {
			label = cleanRegionToken(title)
			known = true
		}
		if label != "" {
			if _, ok := seen[label]; ok {
				continue
			}
			seen[label] = struct{}{}
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ","), known
}
