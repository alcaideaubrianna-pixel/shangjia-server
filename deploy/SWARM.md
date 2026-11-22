# Swarm 生产发布与节点接入

生产运行在腾讯云轻量云 Docker Swarm 集群。Manager 为 `10.3.4.8`，业务通过轻量云公网入口提供服务，日志写入 CLS，指标写入 VictoriaMetrics。

## 发布

```bash
./deploy/tencent-deploy-all.sh <image-tag> publish collector scheduler background media web
```

非 Web 角色并发发布。Web 依次更新 blue、切换路由、更新 green、切回路由。脚本会显示角色耗时、服务副本状态、健康检查和整批耗时。

## 新节点

```bash
./deploy/swarm-node-join.sh ubuntu@<公网IP> <role> /Users/ley/.ssh/id_hk_rsa
```

脚本会安装 Docker、加入 Swarm、设置角色、同步 TCR 凭证、开放监控端口并接入 CLS。Node Exporter、cAdvisor 和 vmagent 通过 Swarm 全局服务及自动发现接入监控。

## 依赖

业务服务使用 CoreDNS 提供的内网域名。当前 DNS 服务运行在 Manager 的 `10.3.4.8:53`，记录包括 Redis、PostgreSQL、OTEL 和 Bot。Redis、PostgreSQL、OTEL 的连通性从实际业务容器路径验证。

## 排障

先检查 Portainer、Swarm 服务副本、Grafana 节点状态、CLS 最近日志和 Redis/PostgreSQL 依赖。collector 需要同时观察实时队列、历史队列、重试原因和媒体依赖等待。
