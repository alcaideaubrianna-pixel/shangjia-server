package sys

import (
	"context"
	"strings"

	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/errors/gerror"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/os/gtime"

	"hotgo/addons/youban_publish/model/input/sysin"
)

// publishJobCreateRequest 描述资料发布入口共享的最小任务参数。
//
// 手动、采集、循环和批次入口只负责组装来源资料、目标频道和操作号，
// 任务事实、Outbox 和频道索引由统一创建入口负责落库。
type publishJobCreateRequest struct {
	Source      gdb.Record
	Channel     telegramJobChannel
	OperationNo string
	Meta        telegramProfilePublishMeta
}

// createPublishJobTx 在同一事务内创建资料发布 Job 和 Outbox。
//
// 调用方不能直接调用 ensureTelegramProfileJobWithMetaTx 后再自行决定
// 是否写 Outbox，避免出现 Job 已存在但没有可靠投递意图的半成品状态。
func (s *sSysPublish) createPublishJobTx(ctx context.Context, tx gdb.TX, req publishJobCreateRequest) (int64, error) {
	if req.Source == nil || req.Source["profile_id"].Int64() <= 0 {
		return 0, gerror.New("资料发布任务缺少资料归属")
	}
	if req.Channel.Id <= 0 {
		return 0, gerror.New("资料发布任务缺少目标频道")
	}
	if strings.TrimSpace(req.OperationNo) == "" {
		return 0, gerror.New("资料发布任务操作号不能为空")
	}
	jobID, err := s.ensureTelegramProfileJobWithMetaTx(ctx, tx, req.Source, req.Channel, req.OperationNo, req.Meta)
	if err != nil {
		return 0, err
	}
	if strings.TrimSpace(req.Meta.InitialSendPhase) != "" {
		if _, err = tx.Model(publishTgJobTable).Ctx(ctx).
			Where("id", jobID).
			Where("status", telegramJobStatusPending).
			Data(g.Map{"send_phase": strings.TrimSpace(req.Meta.InitialSendPhase)}).
			Update(); err != nil {
			return 0, gerror.Wrap(err, "设置资料发布初始阶段失败")
		}
	}
	if err = s.ensureTelegramJobOutboxTx(ctx, tx, jobID, 0); err != nil {
		return 0, err
	}
	return jobID, nil
}

// publishJobCreateMeta 返回入口统一使用的资料发布元数据。
func publishJobCreateMeta(collectEventID, collectSourceID int64, sourceChatID string, sourceMessageID int64, initialPriority int, sendPhase string) telegramProfilePublishMeta {
	return telegramProfilePublishMeta{
		CollectEventId:         collectEventID,
		CollectSourceId:        collectSourceID,
		CollectSourceChatId:    strings.TrimSpace(sourceChatID),
		CollectSourceMessageId: sourceMessageID,
		InitialPriority:        initialPriority,
		InitialSendPhase:       strings.TrimSpace(sendPhase),
	}
}

// createMessagePublishJob 统一消息模板类发布入口的语义名称。
// 消息模板的媒体和文本快照由原有逻辑处理，调用方统一通过此方法创建任务。
func (s *sSysPublish) createMessagePublishJob(
	ctx context.Context,
	template *sysin.MessageTemplateModel,
	channel *messagePushChannel,
	tenantID, accountID, botID int64,
	operationNo string,
	priority int,
	queueName string,
	dispatchStatus string,
	pushMode string,
) (telegramJobRecord, error) {
	if template == nil || channel == nil {
		return telegramJobRecord{}, gerror.New("消息发布任务缺少模板或目标频道")
	}
	if strings.TrimSpace(operationNo) == "" {
		return telegramJobRecord{}, gerror.New("消息发布任务操作号不能为空")
	}
	return s.createMessagePushJobWithOperation(
		ctx, template, channel, tenantID, accountID, botID,
		operationNo, priority, queueName, dispatchStatus, gtime.Now(), pushMode,
	)
}
