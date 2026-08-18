package sys

import (
	"context"
	"strings"

	pdao "hotgo/addons/youban_publish/internal/dao"
	"hotgo/internal/library/publishmedia"
)

func init() {
	publishmedia.Register(resolvePublishedProfileMedia)
}

func resolvePublishedProfileMedia(ctx context.Context, profileIDs []int64) (map[int64][]publishmedia.Item, error) {
	result := make(map[int64][]publishmedia.Item, len(profileIDs))
	if len(profileIDs) == 0 {
		return result, nil
	}
	columns := pdao.YoubanPublishMedia.Columns()
	rows, err := pdao.YoubanPublishMedia.Ctx(ctx).
		Fields(columns.Id, columns.ProfileId, columns.MediaType, columns.FileUrl, columns.StoragePath,
			columns.EditedFileUrl, columns.EditedStoragePath, columns.PosterUrl, columns.PosterStoragePath).
		WhereIn(columns.ProfileId, profileIDs).
		Where(columns.Status, 1).
		WhereNull(columns.DeletedAt).
		OrderAsc(columns.ProfileId).
		OrderAsc(columns.SortIndex).
		OrderAsc(columns.Id).
		All()
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		profileID := row[columns.ProfileId].Int64()
		url := firstProviderValue(row[columns.EditedStoragePath].String(), row[columns.EditedFileUrl].String(),
			row[columns.StoragePath].String(), row[columns.FileUrl].String())
		preview := firstProviderValue(row[columns.PosterStoragePath].String(), row[columns.PosterUrl].String(), url)
		if url == "" {
			continue
		}
		result[profileID] = append(result[profileID], publishmedia.Item{
			ID: row[columns.Id].Int64(), ProfileID: profileID, Type: row[columns.MediaType].String(), URL: url, PreviewURL: preview,
		})
	}
	return result, nil
}

func firstProviderValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
