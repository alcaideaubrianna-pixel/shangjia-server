package sys

import (
	"context"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	idao "hotgo/internal/dao"
)

// BackfillCollectedSourceMetadata repairs only missing durable source message
// coordinates. It is cursor-based so operators can run it in small batches.
func (s *sSysPublish) BackfillCollectedSourceMetadata(ctx context.Context, lastId int64, batchSize int) (scanned, updated int, nextId int64, err error) {
	if batchSize <= 0 || batchSize > 2000 {
		batchSize = 500
	}
	profiles, err := idao.ContentProfile.Ctx(ctx).
		Fields("id,source_key").
		WhereGT("id", lastId).
		Where("source_type", collectProfileSourceType).
		Where("(source_note_id = 0 OR source_note_id IS NULL)").
		WhereLike("source_key", "collect:%").
		WhereNull("deleted_at").
		OrderAsc("id").Limit(batchSize).All()
	if err != nil {
		return 0, 0, lastId, err
	}
	if len(profiles) == 0 {
		return 0, 0, lastId, nil
	}
	nextId = profiles[len(profiles)-1]["id"].Int64()
	scanned = len(profiles)
	keys := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		if key := profile["source_key"].String(); key != "" {
			keys = append(keys, key)
		}
	}
	eventByKey := make(map[string]gdb.Record, len(keys))
	var events []gdb.Record
	if events, err = g.DB().Model("hg_youban_publish_collect_event").Ctx(ctx).
		Fields("source_unique_key,source_message_id,received_at").
		WhereIn("source_unique_key", keys).
		WhereGT("source_message_id", 0).
		OrderDesc("id").All(); err != nil {
		return scanned, 0, nextId, err
	}
	for _, event := range events {
		if _, exists := eventByKey[event["source_unique_key"].String()]; !exists {
			eventByKey[event["source_unique_key"].String()] = event
		}
	}
	if err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		for _, profile := range profiles {
			event, ok := eventByKey[profile["source_key"].String()]
			if !ok {
				continue
			}
			data := g.Map{"source_note_id": event["source_message_id"].Int64(), "updated_at": gtime.Now()}
			if !event["received_at"].IsNil() {
				data["source_created_at"] = event["received_at"]
			}
			result, updateErr := tx.Model(idao.ContentProfile.Table()).Ctx(ctx).
				Where("id", profile["id"].Int64()).Where("(source_note_id = 0 OR source_note_id IS NULL)").Data(data).Update()
			if updateErr != nil {
				return updateErr
			}
			rowsAffected, rowsErr := result.RowsAffected()
			if rowsErr != nil {
				return rowsErr
			}
			updated += int(rowsAffected)
		}
		return nil
	}); err != nil {
		return scanned, updated, nextId, err
	}
	return scanned, updated, nextId, nil
}
