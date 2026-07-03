package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/util/guid"

	"hotgo/addons/youban_publish/model/input/sysin"
	"hotgo/internal/dao"
)

func newPublishProfileUUID() string {
	return guid.S()
}

func normalizeProfileUUID(uuid string) string {
	return strings.TrimSpace(uuid)
}

func hasProfileSelector(id int64, uuid string) bool {
	return id > 0 || normalizeProfileUUID(uuid) != ""
}

func (s *sSysPublish) ensureProfileModelUUID(ctx context.Context, profile *sysin.ProfileModel) error {
	if profile == nil || profile.Id <= 0 || normalizeProfileUUID(profile.Uuid) != "" {
		return nil
	}
	columns := dao.ContentProfile.Columns()
	uuid := newPublishProfileUUID()
	_, err := dao.ContentProfile.Ctx(ctx).
		Where(columns.Id, profile.Id).
		Where("(source_note_uuid IS NULL OR source_note_uuid = '')").
		Data(g.Map{columns.SourceNoteUuid: uuid}).
		Update()
	if err != nil {
		return gerror.Wrap(err, "补齐资料UUID失败")
	}
	profile.Uuid = uuid
	return nil
}

func (s *sSysPublish) ensureProfileListUUID(ctx context.Context, list []*sysin.ProfileModel) error {
	for _, item := range list {
		if err := s.ensureProfileModelUUID(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

func (s *sSysPublish) resolveProfileId(ctx context.Context, id int64, uuid string, tenantId int64, accountId int64) (int64, error) {
	uuid = normalizeProfileUUID(uuid)
	if uuid == "" {
		if id <= 0 {
			return 0, gerror.New("资料UUID不能为空")
		}
		return id, nil
	}
	base, err := s.profileBaseModel(ctx, tenantId, accountId)
	if err != nil {
		return 0, err
	}
	row, err := base.Fields("p.id").Where("p.source_note_uuid", uuid).One()
	if err != nil {
		return 0, gerror.Wrap(err, "读取资料UUID失败")
	}
	profileId := row["id"].Int64()
	if profileId <= 0 {
		return 0, gerror.New("资料不存在或无权操作")
	}
	return profileId, nil
}

func (s *sSysPublish) allowedProfileTargetIds(ctx context.Context, ids []int64, uuids []string, tenantId int64, accountId int64) ([]int64, error) {
	targetIds := make([]int64, 0, len(ids)+len(uuids))
	for _, id := range ids {
		if id > 0 {
			targetIds = append(targetIds, id)
		}
	}
	cleanUUIDs := make([]string, 0, len(uuids))
	for _, uuid := range uuids {
		if uuid = normalizeProfileUUID(uuid); uuid != "" {
			cleanUUIDs = append(cleanUUIDs, uuid)
		}
	}
	if len(cleanUUIDs) > 0 {
		base, err := s.profileBaseModel(ctx, tenantId, accountId)
		if err != nil {
			return nil, err
		}
		rows, err := base.Fields("p.id").WhereIn("p.source_note_uuid", cleanUUIDs).All()
		if err != nil {
			return nil, gerror.Wrap(err, "读取资料UUID失败")
		}
		for _, row := range rows {
			if id := row["id"].Int64(); id > 0 {
				targetIds = append(targetIds, id)
			}
		}
	}
	return uniqueIds(targetIds), nil
}
