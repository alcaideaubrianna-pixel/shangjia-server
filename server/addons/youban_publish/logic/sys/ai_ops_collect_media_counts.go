package sys

import (
	"fmt"

	pdao "hotgo/addons/youban_publish/internal/dao"
)

// Materialize event identity before joining media. UNION prevents counting the
// root twice when it is also a matching child. The explicit root ID and LEFT
// JOIN preserve the old display count for media whose event row is missing.
func aiOpsCollectMediaCountsSQL(eventID int64) (string, []interface{}) {
	e := pdao.YoubanPublishCollectEvent.Columns()
	m := pdao.YoubanPublishCollectEventMedia.Columns()
	// material_role/material_parent_event_id are schema extensions not present
	// in the generated column definitions.
	query := fmt.Sprintf(`WITH event_ids AS MATERIALIZED (
 SELECT CAST(? AS bigint) AS event_id
 UNION
 SELECT child.%s FROM %s child
 WHERE child.material_parent_event_id=? AND child.material_role=?
)
SELECT COUNT(*) FILTER (WHERE COALESCE(e.material_role,'') <> 'verify') AS display_count,
 COUNT(*) FILTER (WHERE e.material_role='verify') AS verify_count
FROM event_ids ids
JOIN %s em ON em.%s=ids.event_id
LEFT JOIN %s e ON e.%s=em.%s`,
		e.Id, pdao.YoubanPublishCollectEvent.Table(),
		pdao.YoubanPublishCollectEventMedia.Table(), m.EventId,
		pdao.YoubanPublishCollectEvent.Table(), e.Id, m.EventId)
	return query, []interface{}{eventID, eventID, collectMaterialRoleVerify}
}
