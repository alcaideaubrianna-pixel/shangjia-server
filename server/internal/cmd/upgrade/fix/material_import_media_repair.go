package fix

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"hotgo/addons/youban_publish/service"
)

// RepairYoubanPublishMaterialImportMissingMedia requeues completed TG import
// groups that have fewer stored media than their source group.
func RepairYoubanPublishMaterialImportMissingMedia(ctx context.Context, accountId int64, groupIds []int64) error {
	return service.SysPublish().RepairMaterialImportMissingMedia(ctx, accountId, groupIds)
}

func ParseMaterialImportRepairGroupIDs(value string) ([]int64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	seen := make(map[int64]struct{})
	for _, raw := range strings.Split(value, ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || id <= 0 {
			return nil, strconv.ErrSyntax
		}
		seen[id] = struct{}{}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}
