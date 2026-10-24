package sys

// A started operation owns the stream until its display and verification are
// complete. Untouched work follows manual publication, collection, then cycles;
// within each class creation time and ID preserve FIFO order. BOT manual
// publications use bot_publish:* and must remain in the manual class; treating
// them as generic work lets stale ai-republish jobs block new user actions.
func telegramChannelScheduleRankSQL(alias string) string {
	p := alias + "."
	op := "LOWER(TRIM(" + p + "operation_no))"
	return "CASE WHEN " + p + "status IN ('sending','confirming') OR " + p + "send_phase IN ('cleanup_processing','display_sending','display_confirmed','verify_sending') THEN 0 WHEN " + op + " LIKE 'profile:%' OR " + op + " LIKE 'bot_publish:%' OR " + op + " LIKE 'batchtext:%' THEN 1 WHEN " + op + " LIKE 'cycle_batch:%' THEN 3 ELSE 2 END"
}

func telegramChannelScheduleOrderSQL(alias string) string {
	return telegramChannelScheduleRankSQL(alias) + ", " + alias + ".created_at, " + alias + ".id"
}

func telegramChannelSchedulePrecedesSQL(left, right string) string {
	l, r := telegramChannelScheduleRankSQL(left), telegramChannelScheduleRankSQL(right)
	return "(" + l + " < " + r + " OR (" + l + " = " + r + " AND (" + left + ".created_at<" + right + ".created_at OR (" + left + ".created_at=" + right + ".created_at AND " + left + ".id<" + right + ".id))))"
}
