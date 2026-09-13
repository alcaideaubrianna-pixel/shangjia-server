package sys

import (
	"context"
	"sort"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

const (
	duplicateScanChunkSize = 500
	duplicateScanBatchSize = 500
)

type duplicateImageRow struct {
	Id        int64  `orm:"id"`
	ProfileId int64  `orm:"profile_id"`
	Md5       string `orm:"md5"`
}

func (s *sSysPublish) AdminNoteDuplicateScan(ctx context.Context, in *sysin.NoteListInp) (*sysin.AdminNoteDuplicateScanModel, error) {
	idsResult, err := s.AdminNoteBatchIds(ctx, in)
	if err != nil {
		return nil, err
	}
	result := &sysin.AdminNoteDuplicateScanModel{Groups: []*sysin.AdminNoteDuplicateGroupModel{}, CandidateTotal: len(idsResult.Ids)}
	if len(idsResult.Ids) == 0 {
		return result, nil
	}

	groupIds := make(map[string][]int64)
	for start := 0; start < len(idsResult.Ids); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(idsResult.Ids) {
			end = len(idsResult.Ids)
		}
		var rows []duplicateImageRow
		err = g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
			Fields("id,profile_id,md5").
			WhereIn("profile_id", idsResult.Ids[start:end]).
			WhereNull("deleted_at").
			WhereIn("media_type", []string{"image", "photo"}).
			Where("purpose IS NULL OR purpose='' OR purpose='display'").
			OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows)
		if err != nil {
			return nil, gerror.Wrap(err, "读取重复资料图片失败")
		}
		mediaByProfile := make(map[int64][]duplicateImageRow, end-start)
		for _, row := range rows {
			mediaByProfile[row.ProfileId] = append(mediaByProfile[row.ProfileId], row)
		}
		for _, profileId := range idsResult.Ids[start:end] {
			signature, complete := duplicateImageSignature(mediaByProfile[profileId])
			if !complete {
				result.IncompleteTotal++
				continue
			}
			groupIds[signature] = append(groupIds[signature], profileId)
		}
	}

	duplicateIds := make([]int64, 0)
	for _, ids := range groupIds {
		if len(ids) > 1 {
			duplicateIds = append(duplicateIds, ids...)
		}
	}
	if len(duplicateIds) == 0 {
		return result, nil
	}

	columns := dao.ContentProfile.Columns()
	profiles := make([]*sysin.AdminNoteDuplicateItemModel, 0, len(duplicateIds))
	for start := 0; start < len(duplicateIds); start += duplicateScanChunkSize {
		end := start + duplicateScanChunkSize
		if end > len(duplicateIds) {
			end = len(duplicateIds)
		}
		var rows []*sysin.AdminNoteDuplicateItemModel
		err = dao.ContentProfile.Ctx(ctx).
			Fields(columns.Id, columns.SourceNoteUuid+" AS uuid", columns.ProfileNo, columns.Title, columns.CreatedAt).
			WhereIn(columns.Id, duplicateIds[start:end]).WhereNull(columns.DeletedAt).Scan(&rows)
		if err != nil {
			return nil, gerror.Wrap(err, "读取重复资料详情失败")
		}
		profiles = append(profiles, rows...)
	}
	profileById := make(map[int64]*sysin.AdminNoteDuplicateItemModel, len(profiles))
	for _, profile := range profiles {
		profileById[profile.Id] = profile
	}

	allGroups := make([]*sysin.AdminNoteDuplicateGroupModel, 0, len(groupIds))
	for signature, ids := range groupIds {
		if len(ids) < 2 {
			continue
		}
		items := make([]*sysin.AdminNoteDuplicateItemModel, 0, len(ids))
		for _, id := range ids {
			if item := profileById[id]; item != nil {
				items = append(items, item)
			}
		}
		if len(items) < 2 {
			continue
		}
		sort.Slice(items, func(i, j int) bool {
			left, right := items[i], items[j]
			leftTime, rightTime := int64(0), int64(0)
			if left.CreatedAt != nil {
				leftTime = left.CreatedAt.Time.UnixNano()
			}
			if right.CreatedAt != nil {
				rightTime = right.CreatedAt.Time.UnixNano()
			}
			if leftTime == rightTime {
				return left.Id > right.Id
			}
			return leftTime > rightTime
		})
		duplicates := append([]*sysin.AdminNoteDuplicateItemModel(nil), items[1:]...)
		allGroups = append(allGroups, &sysin.AdminNoteDuplicateGroupModel{Signature: signature, Keep: items[0], Duplicates: duplicates})
		result.DuplicateTotal += len(duplicates)
	}
	sort.Slice(allGroups, func(i, j int) bool { return allGroups[i].Keep.Id > allGroups[j].Keep.Id })
	result.GroupTotal = len(allGroups)
	result.Groups = duplicateScanBatch(allGroups, duplicateScanBatchSize)
	for _, group := range result.Groups {
		result.BatchTotal += len(group.Duplicates)
	}
	result.HasMore = result.BatchTotal < result.DuplicateTotal
	if err = s.loadDuplicateScanMedia(ctx, result.Groups); err != nil {
		return nil, err
	}
	return result, nil
}

func duplicateScanBatch(groups []*sysin.AdminNoteDuplicateGroupModel, limit int) []*sysin.AdminNoteDuplicateGroupModel {
	if limit <= 0 {
		return []*sysin.AdminNoteDuplicateGroupModel{}
	}
	result := make([]*sysin.AdminNoteDuplicateGroupModel, 0)
	remaining := limit
	for _, group := range groups {
		if group == nil || group.Keep == nil || len(group.Duplicates) == 0 || remaining == 0 {
			continue
		}
		count := len(group.Duplicates)
		if count > remaining {
			count = remaining
		}
		result = append(result, &sysin.AdminNoteDuplicateGroupModel{
			Signature:  group.Signature,
			Keep:       group.Keep,
			Duplicates: append([]*sysin.AdminNoteDuplicateItemModel(nil), group.Duplicates[:count]...),
		})
		remaining -= count
	}
	return result
}

func (s *sSysPublish) loadDuplicateScanMedia(ctx context.Context, groups []*sysin.AdminNoteDuplicateGroupModel) error {
	profileIds := make([]int64, 0)
	items := make(map[int64]*sysin.AdminNoteDuplicateItemModel)
	for _, group := range groups {
		if group == nil || group.Keep == nil {
			continue
		}
		groupItems := append([]*sysin.AdminNoteDuplicateItemModel{group.Keep}, group.Duplicates...)
		for _, item := range groupItems {
			if item == nil || item.Id <= 0 {
				continue
			}
			item.Media = []*sysin.AdminNoteDuplicateMediaModel{}
			items[item.Id] = item
			profileIds = append(profileIds, item.Id)
		}
	}
	if len(profileIds) == 0 {
		return nil
	}
	var rows []struct {
		Id        int64  `orm:"id"`
		ProfileId int64  `orm:"profile_id"`
		FileUrl   string `orm:"file_url"`
	}
	if err := g.DB().Model(publishMediaTable).Safe().Ctx(ctx).
		Fields("id,profile_id,file_url").WhereIn("profile_id", uniqueIds(profileIds)).
		WhereNull("deleted_at").WhereIn("media_type", []string{"image", "photo"}).
		Where("purpose IS NULL OR purpose='' OR purpose='display'").
		OrderAsc("profile_id").OrderAsc("sort_index").OrderAsc("id").Scan(&rows); err != nil {
		return gerror.Wrap(err, "读取重复资料图片预览失败")
	}
	for _, row := range rows {
		if item := items[row.ProfileId]; item != nil {
			item.Media = append(item.Media, &sysin.AdminNoteDuplicateMediaModel{Id: row.Id, FileUrl: row.FileUrl})
		}
	}
	return nil
}

func duplicateImageSignature(rows []duplicateImageRow) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		value := strings.ToLower(strings.TrimSpace(row.Md5))
		if value == "" {
			return "", false
		}
		values = append(values, value)
	}
	sort.Strings(values)
	return collectHash(strings.Join(values, "|")), true
}
