# 8. HotGo 实现规范

- 使用 GoFrame v2 的 `gdb、gerror、gctx、gtime`；
- Controller 只透传，业务写在 logic，数据库访问通过 DAO/Model；
- 业务错误使用中文 `gerror.New/Wrap`；
- 缓存通过 `cache.Instance()` 或基础设施层统一封装，业务代码不得直接调用 Redis；
- 队列 Topic 和缓存 Key 集中定义；Key 必须带租户和频道；
- 定时任务通过 `cron.Register` 注册，消费者通过 `queue.RegisterConsumer` 注册；
- 新逻辑必须有中文注释、结构化日志和可运行回放；
- DAO 生成文件禁止手改；
- 不以单元测试替代 PG、Redis、TG Fake 的真实链路验收。
