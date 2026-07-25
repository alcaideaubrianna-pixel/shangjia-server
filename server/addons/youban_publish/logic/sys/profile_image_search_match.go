package sys

import (
	"context"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
)

func (s *sSysPublish) attachProfileImageSearchMatches(ctx context.Context, notes []*sysin.NoteModel, matches []publishProfilePHashDistance) error {
	mediaIds := make([]int64, 0, len(matches))
	profileMedia := make(map[int64]int64, len(matches))
	for _, match := range matches {
		if match.ProfileId <= 0 || match.MediaId <= 0 {
			continue
		}
		profileMedia[match.ProfileId] = match.MediaId
		mediaIds = append(mediaIds, match.MediaId)
	}
	if len(mediaIds) == 0 {
		return nil
	}
	mediaIds = uniqueIds(mediaIds)
	var media []*sysin.MediaModel
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		WhereIn("id", mediaIds).
		WhereNull("deleted_at").
		Scan(&media); err != nil {
		return gerror.Wrap(err, "获取图片搜索命中媒体失败")
	}
	normalizeMediaListFileURL(media)
	byId := make(map[int64]*sysin.MediaModel, len(media))
	for _, item := range media {
		if item != nil && item.Id > 0 {
			byId[item.Id] = item
		}
	}
	for _, note := range notes {
		if note == nil {
			continue
		}
		if mediaId := profileMedia[note.Id]; mediaId > 0 {
			note.MatchedMedia = byId[mediaId]
		}
	}
	return nil
}
