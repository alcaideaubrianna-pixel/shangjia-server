package sys

import "context"

// StartWorker 将更新消费交给既有后台Worker角色，复用Asynq幂等和重试，不启动独占Bot运行时。
func (s *sGateway) StartWorker(ctx context.Context) {
	s.startUpdateQueue(ctx)
}
