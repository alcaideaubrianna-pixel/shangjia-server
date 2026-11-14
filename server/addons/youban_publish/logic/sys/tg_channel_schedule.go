package sys

// A started operation owns the stream until its display and verification are
// complete. Untouched work uses the same effective priority as queue selection;
// creation time and ID preserve FIFO order within one priority.
func telegramChannelScheduleRankSQL(alias string) string {
	p := alias
	if p != "" {
		p += "."
	}
	// A realtime collect task (priority below the history boundary) must be
	// able to pass an older profile_cycle backlog in the same channel. Keep
	// this explicit here because channel ordering is also used by the SQL
	// recovery path, independently of queue-name classification.
	return "CASE WHEN " + p + "status IN ('sending','confirming','unknown') OR " + p + "send_phase IN ('cleanup_processing','display_sending','display_confirmed','verify_sending') THEN 0 " +
		"WHEN LEFT(LOWER(TRIM(" + p + "operation_no)),8)='collect:' AND COALESCE(" + p + "priority,0) < 90 THEN 10 " +
		"WHEN " + p + "collect_source_id > 0 AND LEFT(LOWER(TRIM(" + p + "operation_no)),14)='profile_cycle:' THEN 10 " +
		"ELSE " + telegramJobEffectivePrioritySQL(alias) + " END"
}

func telegramChannelScheduleOrderSQL(alias string) string {
	return telegramChannelScheduleRankSQL(alias) + ", " + alias + ".created_at, " + alias + ".id"
}

func telegramChannelSchedulePrecedesSQL(left, right string) string {
	l, r := telegramChannelScheduleRankSQL(left), telegramChannelScheduleRankSQL(right)
	return "(" + l + " < " + r + " OR (" + l + " = " + r + " AND (" + left + ".created_at<" + right + ".created_at OR (" + left + ".created_at=" + right + ".created_at AND " + left + ".id<" + right + ".id))))"
}
