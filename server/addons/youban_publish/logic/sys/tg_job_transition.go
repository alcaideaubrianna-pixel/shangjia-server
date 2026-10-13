package sys

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"
)

const (
	telegramJobStatusPending    = "pending"
	telegramJobStatusSending    = "sending"
	telegramJobStatusRetryWait  = "failed_retry"
	telegramJobStatusConfirming = "unknown"
	telegramJobStatusSent       = "sent"
	telegramJobStatusFailed     = "failed"
	telegramJobStatusSuperseded = "superseded"
)

var telegramJobAllowedTransitions = map[string]map[string]struct{}{
	telegramJobStatusPending: {
		telegramJobStatusSending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusSent: {},
		telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusSending: {
		telegramJobStatusPending: {}, telegramJobStatusRetryWait: {}, telegramJobStatusConfirming: {},
		telegramJobStatusSent: {}, telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusRetryWait: {
		telegramJobStatusPending: {}, telegramJobStatusSending: {}, telegramJobStatusRetryWait: {},
		telegramJobStatusSent: {}, telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusConfirming: {
		telegramJobStatusRetryWait: {}, telegramJobStatusSent: {}, telegramJobStatusFailed: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusFailed: {
		telegramJobStatusRetryWait: {}, telegramJobStatusSent: {}, telegramJobStatusSuperseded: {},
	},
	telegramJobStatusSent: {
		telegramJobStatusSuperseded: {},
	},
}

type telegramJobTransition struct {
	JobId        int64
	From         []string
	To           string
	RetryDelay   time.Duration
	ErrorMessage string
	Additional   g.Map
}

func validateTelegramJobTransition(from []string, to string) error {
	to = strings.TrimSpace(to)
	if to == "" || len(from) == 0 {
		return gerror.New("TG任务状态迁移参数不完整")
	}
	for _, source := range from {
		source = strings.TrimSpace(source)
		if source == "" {
			return gerror.New("TG任务来源状态不能为空")
		}
		if _, ok := telegramJobAllowedTransitions[source][to]; !ok {
			return gerror.Newf("TG任务状态不允许从 %s 迁移到 %s", source, to)
		}
	}
	return nil
}

// transitionTelegramJob is the only write primitive for business status.
// Callers may still update lease/phase metadata directly when status is unchanged.
func (s *sSysPublish) transitionTelegramJob(ctx context.Context, in telegramJobTransition) (bool, error) {
	if in.JobId <= 0 {
		return false, gerror.New("TG任务ID不能为空")
	}
	if err := validateTelegramJobTransition(in.From, in.To); err != nil {
		return false, err
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return false, err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		Where("id", in.JobId).WhereIn("status", from).Data(data).Update()
	if err != nil {
		return false, gerror.Wrapf(err, "TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, gerror.Wrap(err, "确认TG任务状态迁移结果失败")
	}
	return affected > 0, nil
}

func (s *sSysPublish) transitionTelegramJobs(ctx context.Context, jobIds []int64, in telegramJobTransition) (int64, error) {
	jobIds = uniqueIds(jobIds)
	if len(jobIds) == 0 {
		return 0, nil
	}
	from, data, err := telegramJobTransitionUpdate(in)
	if err != nil {
		return 0, err
	}
	result, err := g.DB().Model(publishTgJobTable).Safe().Ctx(ctx).
		WhereIn("id", jobIds).WhereIn("status", from).Data(data).Update()
	if err != nil {
		return 0, gerror.Wrapf(err, "批量TG任务状态迁移失败：%s -> %s", strings.Join(from, ","), in.To)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, gerror.Wrap(err, "确认批量TG任务状态迁移结果失败")
	}
	return affected, nil
}

func telegramJobTransitionUpdate(in telegramJobTransition) ([]string, g.Map, error) {
	if err := validateTelegramJobTransition(in.From, in.To); err != nil {
		return nil, nil, err
	}
	from := append([]string(nil), in.From...)
	for index := range from {
		from[index] = strings.TrimSpace(from[index])
	}
	sort.Strings(from)
	now := gtime.Now()
	data := telegramJobStateUpdateData(strings.TrimSpace(in.To), in.RetryDelay, now)
	if in.ErrorMessage != "" || in.To == telegramJobStatusSent {
		data["error_message"] = strings.TrimSpace(in.ErrorMessage)
	}
	for field, value := range in.Additional {
		if strings.EqualFold(strings.TrimSpace(field), "status") {
			return nil, nil, gerror.New("TG任务附加更新字段不能包含status")
		}
		data[field] = value
	}
	return from, data, nil
}
