package fix

import (
	"context"

	"hotgo/addons/youban_publish/service"
)

// RepairYoubanPublishMaterialImportMissingMedia requeues completed TG import
// groups that have fewer stored media than their source group.
func RepairYoubanPublishMaterialImportMissingMedia(ctx context.Context, accountId int64) error {
	return service.SysPublish().RepairMaterialImportMissingMedia(ctx, accountId)
}
