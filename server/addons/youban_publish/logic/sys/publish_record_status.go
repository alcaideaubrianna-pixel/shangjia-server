package sys

import (
	"fmt"
	"strings"

	publishmodel "hotgo/addons/youban_publish/model"
)

// Both Go projection writes and SQL reads use this mapping. Dispatch state and
// a profile's current operation are deliberately not publication job results.
var publishRecordJobStatusMappings = []struct {
	Job    JobStatus
	Record publishmodel.PublishRecordStatus
}{
	{JobStatusPending, publishmodel.PublishRecordStatusPending},
	{JobStatusSending, publishmodel.PublishRecordStatusSending},
	{JobStatusSent, publishmodel.PublishRecordStatusSuccess},
	{JobStatusRetryWait, publishmodel.PublishRecordStatusRetryWait},
	{JobStatusFailed, publishmodel.PublishRecordStatusFailed},
	{JobStatusSuperseded, publishmodel.PublishRecordStatusSuperseded},
	{JobStatusUnknown, publishmodel.PublishRecordStatusUnknown},
}

func publishRecordLegacyStatusSQL() string {
	var sql strings.Builder
	sql.WriteString("CASE TRIM(COALESCE(l.status,''))")
	// "sent" is the old record/API spelling of a successful job.
	fmt.Fprintf(&sql, " WHEN '%s' THEN '%s'", JobStatusSent, publishmodel.PublishRecordStatusSuccess)
	for _, status := range []publishmodel.PublishRecordStatus{
		publishmodel.PublishRecordStatusPending, publishmodel.PublishRecordStatusSending,
		publishmodel.PublishRecordStatusSuccess, publishmodel.PublishRecordStatusRetryWait,
		publishmodel.PublishRecordStatusFailed, publishmodel.PublishRecordStatusSuperseded,
		publishmodel.PublishRecordStatusUnknown, publishmodel.PublishRecordStatusCancelled,
		publishmodel.PublishRecordStatusDeleted,
	} {
		fmt.Fprintf(&sql, " WHEN '%s' THEN '%s'", status, status)
	}
	fmt.Fprintf(&sql, " ELSE '%s' END", publishmodel.PublishRecordStatusUnknown)
	return sql.String()
}

func publishRecordStatusSQL() string {
	var sql strings.Builder
	sql.WriteString("CASE WHEN j.id IS NULL THEN " + publishRecordLegacyStatusSQL() + " ELSE CASE j.status")
	for _, mapping := range publishRecordJobStatusMappings {
		fmt.Fprintf(&sql, " WHEN '%s' THEN '%s'", mapping.Job, mapping.Record)
	}
	fmt.Fprintf(&sql, " ELSE '%s' END END", publishmodel.PublishRecordStatusUnknown)
	return sql.String()
}

var publishRecordStatusExpression = publishRecordStatusSQL()

// An obsolete snapshot message must not say "waiting" for a successful job.
// Build defaults with the same typed formatter used for writes, so keyword
// filtering and the returned message agree even before old snapshots are fixed.
func publishRecordDefaultMessageSQL() string {
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
	statusCase := func(action string) string {
		var sql strings.Builder
		sql.WriteString("CASE (" + publishRecordStatusExpression + ")")
		for _, status := range []publishmodel.PublishRecordStatus{
			publishmodel.PublishRecordStatusPending, publishmodel.PublishRecordStatusSending,
			publishmodel.PublishRecordStatusSuccess, publishmodel.PublishRecordStatusRetryWait,
			publishmodel.PublishRecordStatusFailed, publishmodel.PublishRecordStatusSuperseded,
			publishmodel.PublishRecordStatusUnknown, publishmodel.PublishRecordStatusCancelled,
			publishmodel.PublishRecordStatusDeleted,
		} {
			fmt.Fprintf(&sql, " WHEN '%s' THEN ", status)
			if status == publishmodel.PublishRecordStatusSuccess && action != publishSuccessTypeQuick {
				sql.WriteString("CASE l.action")
				for _, successAction := range []string{
					publishSuccessTypeCollect, publishSuccessTypeCycle, publishSuccessTypeFull,
				} {
					fmt.Fprintf(&sql, " WHEN %s THEN %s", quote(successAction),
						quote(publishJobRecordMessage(successAction, status)))
				}
				sql.WriteString(" ELSE " + quote(publishJobRecordMessage(publishSuccessTypeProfile, status)) + " END")
			} else {
				sql.WriteString(quote(publishJobRecordMessage(action, status)))
			}
		}
		sql.WriteString(" ELSE " + quote(publishJobRecordMessage(action, publishmodel.PublishRecordStatusUnknown)) + " END")
		return sql.String()
	}
	return "CASE WHEN l.action=" + quote(publishSuccessTypeQuick) + " THEN " +
		statusCase(publishSuccessTypeQuick) + " ELSE " + statusCase(publishSuccessTypeProfile) + " END"
}

// Detailed job failures win over a generic status message.
var publishRecordMessageExpression = "CASE WHEN j.status IN ('" +
	JobStatusFailed.String() + "','" + JobStatusRetryWait.String() + "','" +
	JobStatusUnknown.String() + "','" + JobStatusSuperseded.String() +
	"') AND COALESCE(j.error_message,'')<>'' THEN j.error_message" +
	" WHEN (j.id IS NOT NULL AND (" + publishRecordLegacyStatusSQL() + ")<> (" +
	publishRecordStatusExpression + ")) OR COALESCE(l.message,'')='' THEN (" +
	publishRecordDefaultMessageSQL() + ") ELSE l.message END"
