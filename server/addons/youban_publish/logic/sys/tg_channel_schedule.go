package sys

// A started operation owns the stream until its display and verification are
// complete. Untouched work uses the same effective priority as queue selection;
// creation time and ID preserve FIFO order within one priority.
func telegramChannelScheduleRankSQL(alias string) string {
	p := alias
	if p != "" {
		p += "."
	}
	return "CASE WHEN " + telegramChannelBlockingJobStateSQL(alias) + " THEN 0 ELSE 1 END, " +
		telegramJobEffectivePrioritySQL(alias)
}

func telegramChannelScheduleOrderSQL(alias string) string {
	return telegramChannelScheduleRankSQL(alias) + ", " + alias + ".created_at, " + alias + ".id"
}

func telegramChannelSchedulePrecedesSQL(left, right string) string {
	l, r := telegramChannelScheduleRankSQL(left), telegramChannelScheduleRankSQL(right)
	return "((" + l + ") < (" + r + ") OR ((" + l + ") = (" + r + ") AND (" + left + ".created_at<" + right + ".created_at OR (" + left + ".created_at=" + right + ".created_at AND " + left + ".id<" + right + ".id))))"
}
