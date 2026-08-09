package sys

import "strings"

func (s *sSysPublish) telegramJobPriority(job telegramJobRecord) int {
	return telegramJobPriorityValue(job)
}

func isTelegramUrgentJob(job telegramJobRecord) bool {
	return telegramJobPriorityValue(job) <= tgJobPriorityUrgent
}

func telegramJobPriorityValue(job telegramJobRecord) int {
	operationNo := strings.ToLower(strings.TrimSpace(job.OperationNo))
	if strings.HasPrefix(operationNo, "profile:") {
		return tgJobPriorityUrgent
	}
	if job.Priority > 0 && job.Priority != 100 {
		return job.Priority
	}
	if strings.HasPrefix(operationNo, "full_push:") || strings.HasPrefix(operationNo, "cycle_batch:") {
		return tgJobPriorityBulk
	}
	return tgJobPriorityDefault
}

func telegramQueueNameByPriority(priority int) string {
	switch {
	case priority <= tgJobPriorityUrgent:
		return tgQueueNameUrgent
	case priority >= tgJobPriorityBulk:
		return tgQueueNameBulk
	default:
		return tgQueueNameDefault
	}
}
