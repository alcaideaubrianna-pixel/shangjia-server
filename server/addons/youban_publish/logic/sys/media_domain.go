package sys

import (
	"strings"

	"github.com/gogf/gf/v2/database/gdb"

	"hotgo/addons/youban_publish/model/input/sysin"
)

const (
	mediaEditStatusEdited = "edited"
	mediaEditStatusRaw    = "raw"
	tgCacheStatusInvalid  = "invalid"
	tgCacheStatusValid    = "valid"
)

type mediaAsset struct {
	AttachmentId int64
	FileUrl      string
	StoragePath  string
	Hash         string
}

type profileMedia struct {
	Id                   int64
	AttachmentId         int64
	OriginalAttachmentId int64
	EditedAttachmentId   int64
	FileUrl              string
	OriginalFileUrl      string
	EditedFileUrl        string
	StoragePath          string
	OriginalStoragePath  string
	EditedStoragePath    string
	Md5                  string
	EditStatus           string
	TgFileId             string
	TgThumbFileId        string
	TgCacheAssetHash     string
	TgCacheStatus        string
}

func newProfileMediaFromRecord(row gdb.Record) profileMedia {
	return profileMedia{
		Id:                   row["id"].Int64(),
		AttachmentId:         row["attachment_id"].Int64(),
		OriginalAttachmentId: row["original_attachment_id"].Int64(),
		EditedAttachmentId:   row["edited_attachment_id"].Int64(),
		FileUrl:              row["file_url"].String(),
		OriginalFileUrl:      row["original_file_url"].String(),
		EditedFileUrl:        row["edited_file_url"].String(),
		StoragePath:          row["storage_path"].String(),
		OriginalStoragePath:  row["original_storage_path"].String(),
		EditedStoragePath:    row["edited_storage_path"].String(),
		Md5:                  row["md5"].String(),
		EditStatus:           row["edit_status"].String(),
		TgFileId:             row["tg_file_id"].String(),
		TgThumbFileId:        row["tg_thumb_file_id"].String(),
		TgCacheAssetHash:     row["tg_cache_asset_hash"].String(),
		TgCacheStatus:        row["tg_cache_status"].String(),
	}
}

func newProfileMediaFromModel(item *sysin.MediaModel) profileMedia {
	if item == nil {
		return profileMedia{}
	}
	return profileMedia{
		Id:                   item.Id,
		AttachmentId:         item.AttachmentId,
		OriginalAttachmentId: item.OriginalAttachmentId,
		EditedAttachmentId:   item.EditedAttachmentId,
		FileUrl:              item.FileUrl,
		OriginalFileUrl:      item.OriginalFileUrl,
		EditedFileUrl:        item.EditedFileUrl,
		StoragePath:          item.StoragePath,
		OriginalStoragePath:  item.OriginalStoragePath,
		EditedStoragePath:    item.EditedStoragePath,
		Md5:                  item.Md5,
		EditStatus:           item.EditStatus,
	}
}

func (m profileMedia) EffectiveAsset() mediaAsset {
	if strings.TrimSpace(m.EditStatus) == mediaEditStatusEdited && (strings.TrimSpace(m.EditedStoragePath) != "" || strings.TrimSpace(m.EditedFileUrl) != "" || m.EditedAttachmentId > 0) {
		return mediaAsset{
			AttachmentId: positiveInt64(m.EditedAttachmentId, m.AttachmentId),
			FileUrl:      firstNonEmpty(m.EditedFileUrl, m.FileUrl),
			StoragePath:  firstNonEmpty(m.EditedStoragePath, m.StoragePath),
			Hash:         mediaAssetHash(m.Md5, firstNonEmpty(m.EditedStoragePath, m.StoragePath), firstNonEmpty(m.EditedFileUrl, m.FileUrl)),
		}
	}
	return mediaAsset{
		AttachmentId: positiveInt64(m.OriginalAttachmentId, m.AttachmentId),
		FileUrl:      firstNonEmpty(m.OriginalFileUrl, m.FileUrl),
		StoragePath:  firstNonEmpty(m.OriginalStoragePath, m.StoragePath),
		Hash:         mediaAssetHash(m.Md5, firstNonEmpty(m.OriginalStoragePath, m.StoragePath), firstNonEmpty(m.OriginalFileUrl, m.FileUrl)),
	}
}

func (m profileMedia) ValidTgFileId(asset mediaAsset) string {
	if strings.TrimSpace(m.TgCacheStatus) != tgCacheStatusValid {
		return ""
	}
	if strings.TrimSpace(m.TgCacheAssetHash) == "" || strings.TrimSpace(m.TgCacheAssetHash) != strings.TrimSpace(asset.Hash) {
		return ""
	}
	return strings.TrimSpace(m.TgFileId)
}

func mediaAssetHash(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func positiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
