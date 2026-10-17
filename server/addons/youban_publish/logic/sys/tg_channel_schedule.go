package sys

// A started operation owns the stream until its display and verification are
// complete. Only untouched work may be overtaken by an interactive publication.
func telegramChannelScheduleRankSQL(alias string) string {
	p := alias + "."
	op := "LOWER(TRIM(" + p + "operation_no))"
	return "CASE WHEN " + p + "status IN ('sending','confirming') OR " + p + "send_phase IN ('cleanup_processing','display_sending','display_confirmed','verify_sending') THEN 0 WHEN " + op + " LIKE 'profile:%' OR " + op + " LIKE 'batchtext:%' THEN 1 ELSE 2 END"
}

func telegramChannelScheduleOrderSQL(alias string) string {
	return telegramChannelScheduleRankSQL(alias) + ", " + alias + ".created_at, " + alias + ".id"
}

func telegramChannelSchedulePrecedesSQL(left, right string) string {
	l, r := telegramChannelScheduleRankSQL(left), telegramChannelScheduleRankSQL(right)
	return "(" + l + " < " + r + " OR (" + l + " = " + r + " AND (" + left + ".created_at<" + right + ".created_at OR (" + left + ".created_at=" + right + ".created_at AND " + left + ".id<" + right + ".id))))"
}

