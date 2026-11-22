# 腾讯云 TKE 红蓝发布

目标集群为新加坡 `Xiaohuiji-Cluster`（`cls-7wx2ohw2`），业务命名空间为 `xiaohuiji`。GitHub Actions 构建并推送镜像，生产切流由发布人通过本地 `tccli` 和 `kubectl` 执行。日常排障见 [腾讯云生产排障手册](../docs/production-debugging.md)。

[腾讯云发布脚本](tencent-deploy-all.sh) 支持 Web 红蓝切流和后台角色交接。下方手动步骤用于演练、检查和故障恢复。彩色工作负载和切流流程须先通过独立环境演练。

## 连接与权限

默认使用已确认可访问生产 TKE 的 kubeconfig。缺少时，用本机 `tccli` 获取临时配置，选择当前可达的内网或公网 API。不通过临时放开公网访问来解决连通问题。

```bash
# 在专用 Bash 会话运行
umask 077
tke_config=$(mktemp)
trap 'rm -f -- "$tke_config"' EXIT
tccli tke DescribeClusterKubeconfig --region ap-singapore \
  --ClusterId cls-7wx2ohw2 --IsExtranet false \
  | jq -er '(.Response // .).Kubeconfig | select(type == "string" and length > 0)' > "$tke_config"
k=(kubectl --kubeconfig "$tke_config" --namespace xiaohuiji)
"${k[@]}" --request-timeout=10s get deployment,service -o wide
```

已有配置时使用 `k=(kubectl --kubeconfig "$TENCENT_KUBECONFIG" --context "$TENCENT_KUBE_CONTEXT" --namespace xiaohuiji)` 并填入已核对的路径和 context。集群 API 超时先确认接入网络、所选 API 端点、访问白名单和 RBAC。必须能读取 Deployment、Pod、Service、EndpointSlice 和事件，且确认 `web` Service 确实承接线上 CLB 入口，才继续。

## 角色和发布边界

| 角色 | 启动命令 | 切换方式 |
|---|---|---|
| web | 当前按在用 Deployment 核对，纯 HTTP 候选可评估 `web` | 两套独立 Deployment 和一条稳定入口，健康验收后切流 |
| collector | `collector-worker` | 独立预发验收，按队列迁移或交接 |
| background | `background-worker` | 同上 |
| media | `media-worker` | 同上 |
| publish | `publish-worker` | 同上，核对成功回执与未知结果 |
| scheduler | `scheduler` | 单实例交接，验证调度租约和最近任务结果 |
| account | `account` | 严格单实例交接，检查旧进程和会话释放；提前安排短暂停顿 |

脚本继承现用 Web 启动参数。旧发布使用的 `runtime` 会启动 HTTP 和 Cron。候选 Web 并行前须根据实际 ENV 和镜像验证其不会启动重复消费者、Cron、Bot 控制面或独占会话。若另行迁移到 `web` 命令，先验证它覆盖线上鉴权、Webhook、WebSocket 与所有现用入口，并确认移出的运行职责有明确归属。验证完成后设置 `TENCENT_WEB_PARALLEL_VERIFIED=true` 启用 Web 红蓝。默认对现用 Web 执行 `maxSurge=0`、`maxUnavailable=1` 的滚动发布，要求至少两个在线且就绪的副本。滚动失败尝试恢复原 Pod 模板，发布或回滚验证成功后恢复原滚动策略。

## 使用脚本

```bash
./deploy/tencent-deploy-all.sh <image-tag> web
TENCENT_WEB_PARALLEL_VERIFIED=true ./deploy/tencent-deploy-all.sh <image-tag> web
./deploy/tencent-deploy-all.sh <image-tag> publish media
./deploy/tencent-deploy-all.sh --env-file /path/to/env.json account
```

- 镜像模式锁定 TCR 摘要。环境变量模式保留现用镜像、启动参数和副本数。生产候选关闭启动时的空库初始化。所有指定角色并发发布。省略角色时并发发布全部七个角色。有先后依赖的变更应拆成独立命令。
- Web 候选使用 Kubernetes Pod proxy 分别访问 8000 端口的 `/readyz`，需要 `pods/proxy` 访问权限。脚本核对生产 Service 端点，切流后默认观察 30 秒，公网健康探测默认使用生产 `/readyz`。发布人可通过 `TENCENT_WEB_SMOKE_SCRIPT` 提供已演练的可执行业务探测，入口参数分别为 `preview <pod>` 和 `live <ready-url>`，返回非零会阻止切流或触发回切。首次生产切换前应验收代表性的业务接口。
- 后台角色从同一角色的当前在线 Deployment 建立零副本候选，先等待旧 Pod 完全退出，再启动新色。Scheduler 和 Account 各自执行单实例交接，且要求旧节点保持 Ready；需要读取相应 Node 的权限。部署失败会尝试恢复该角色，其他角色继续完成各自的部署。
- 脚本通过集群内 `youban-production-release` ConfigMap 串行化整批发布。Ctrl+C 快速退出并保留锁，已经被 Kubernetes 接受的变更仍可能继续。自动恢复失败也会保留锁。确认发布进程已停止、检查各角色实际副本并恢复到稳定状态后，才由值班人员移除锁。手动切流也须遵守同一发布窗口。

## 发布前核对

1. 记录发布窗口、目标角色、当前镜像摘要和回滚摘要。查询 TCR 标签并核对唯一摘要，确认准备好的发布包不会删除旧版本、修改线上 Secret 或变更 CLB/DNS。
2. 检查目标版本与旧版共享库表、配置、Redis key 与队列消息时的双向兼容性。数据库采用扩展 → 双版本兼容 → 收敛。DDL、回填和索引单独执行，候选 Pod 启动时保持自动初始化关闭。存在不可回滚的数据变更时拆分发版。
3. 检查 PostgreSQL 锁等待、Redis 超时、队列积压、生产 5xx 和 P95，记录发布前基线。只在基线正常时切换。预留两套 Web 的 CPU、内存、Pod IP、PG 连接池和 Redis 连接容量。
4. 同一时段只运行一条生产发布流程。对目标资源执行服务端 dry run，核对镜像、Secret 引用和网络路径。候选清单里只引用 Secret，防止 `kubectl diff` 输出秘密值。

```bash
tag='<Git 提交标签>'
tccli tcr DescribeImagePersonal --region ap-singapore \
  --RepoName xiaohuiji-cc/xiaohuiji-package --Tag "$tag" \
  | jq -er --arg tag "$tag" '
      [(.Response // .).Data.TagInfo[]? | select(.TagName == $tag) | .TagId
       | select(type == "string" and test("^sha256:[0-9a-f]{64}$"))]
      | if length == 1 then .[0] else error("找不到唯一镜像摘要") end'
```

使用 `sgccr.ccs.tencentyun.com/xiaohuiji-cc/xiaohuiji-package@sha256:<digest>`。上线后再核对 Pod 的 `imageID`；多架构镜像的容器摘要可能对应平台 manifest，按 TCR 镜像索引核对。 [CI 镜像构建](../docs/tencent-ci-build.md)

## Web 红蓝部署与切流

### 1. 创建非在线颜色

- 稳定入口使用现有 `Service/web`，保留名称、端口、ClusterIP 与 CLB/Ingress 关联。候选颜色通过单独的 ClusterIP Service 探测，禁止配置生产外部入口。
- 使用 `web-red` 和 `web-blue` 两个 Deployment。两色 selector 分别为 `app.kubernetes.io/name=xiaohuiji-web,rollout-color=red` 和相同 app 名搭配 `rollout-color=blue`。Pod 同时保留 `app.kubernetes.io/component=web` 供可观测性核对。首次接入时查看现网 Deployment 与 Service 的**完整** selector，候选标签必须与在线 selector 不相交。若在线 selector 仅有 `component=web` 等宽泛条件，先调整方案，禁止创建会被线上 Service 提前选中的候选 Pod。
- 每次从已审查的声明式清单部署非在线颜色，设置两个 Web 副本、资源 requests/limits、startup/readiness/liveness probe、优雅退出宽限期。startup probe 覆盖初始化，liveness 只探进程自身，readiness 验证依赖。按现网策略校验 Secret、NetworkPolicy、TCR 拉取和内网 DNS。
- 准备 `candidate.yaml` 时只包含候选 Deployment 和候选 Service，如 `web-blue-preview`。先以 Git 中或安全导出的当前 Pod 模板为参考，去掉 `status`、UID、resourceVersion、managedFields 等服务端字段，显式填入候选镜像与标签。禁止改现有 Deployment 的不可变 selector。 [Deployment selector](https://kubernetes.io/docs/concepts/workloads/controllers/deployment/)

```bash
candidate=blue   # 根据实际 Service selector 选择非在线颜色
"${k[@]}" apply --dry-run=server -f candidate.yaml
"${k[@]}" diff -f candidate.yaml  # 退出码 1 表示有差异；大于 1 表示错误
# 确认差异只包含候选资源后执行
"${k[@]}" apply -f candidate.yaml
"${k[@]}" rollout status "deployment/web-$candidate" --timeout=5m
"${k[@]}" get endpointslices -l "kubernetes.io/service-name=web-$candidate-preview" -o wide
"${k[@]}" port-forward "service/web-$candidate-preview" 18000:8000
```

从另一个终端测试 `http://127.0.0.1:18000/readyz`，再验证代表性的登录态只读接口。port-forward 单次只落到一个 Pod，需要分别核对两副本。另从集群内验证候选 Service DNS、就绪端点、响应和依赖，确认 Grafana 与 CLS 能区分两色。测试 WebSocket 和 webhook 路由时使用约定的测试数据，避免发送真实任务。

### 2. 原子替换 Service selector

切流前在同一个 Bash 会话启动入口探针，记录状态码和耗时。保存旧 selector 到专用变量，使用 JSON Patch 的 `test` 防止覆盖并发变更。如果候选端点不足、原生产端点健康异常或基线恶化，停止切流。

```bash
svc=$("${k[@]}" get service web -o json)
service_uid=$(jq -er '.metadata.uid' <<< "$svc")
previous_selector=$(jq -ce '.spec.selector | select(type == "object" and length > 0)' <<< "$svc")
next_selector=$(jq -cn --arg color "$candidate" \
  '{"app.kubernetes.io/name":"xiaohuiji-web","rollout-color":$color}')

switch_web() {
  local from="$1" to="$2"
  jq -cn --arg uid "$service_uid" --argjson from "$from" --argjson to "$to" '
    [{op:"test",path:"/metadata/uid",value:$uid},
     {op:"test",path:"/spec/selector",value:$from},
     {op:"replace",path:"/spec/selector",value:$to}]' \
  | "${k[@]}" patch service web --type=json --patch-file=/dev/stdin
}
switch_web "$previous_selector" "$next_selector"
"${k[@]}" get endpointslices -l kubernetes.io/service-name=web -o wide
```

- 发布记录保存 Service UID、前后 selector、旧 Deployment 名及摘要，支持跨终端回滚。确认 EndpointSlice 的 ready 端点全部指向新色，再查 CLB 后端及真实域名的接口响应、CLS、P95/5xx。Service 更新后，EndpointSlice 与 CLB 的更新是异步的。必须演练连接切换，尤其是 WebSocket、保活长连接与 CLB 更新窗口。 [EndpointSlice 状态](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/)
- 旧色继续保持就绪，等待已有连接正常排空并覆盖至少一个业务验证窗口。任一关键接口失败、就绪端点缺失、5xx 或延迟持续恶化时立即回滚。验收后再按容量计划降旧色为零，保留可回滚配置和摘要。

### 3. 回滚

先确认旧色仍有就绪 Pod。已缩容时，先按原副本数拉起并确认就绪。确认旧版本仍兼容生产数据，再在原 Bash 会话执行：

```bash
switch_web "$next_selector" "$previous_selector"
"${k[@]}" get endpointslices -l kubernetes.io/service-name=web -o wide
curl -fsS --max-time 8 https://api.xiaohuiji.cc/readyz
```

如已换会话，从发布记录恢复选择器和 Service UID。回切后验证真实业务请求、CLB 后端及 P95/5xx，待旧色稳定后再处置候选。切流后新增的数据按业务流程对账，禁止用数据库恢复覆盖它们。

## Worker 与独占角色交接

普通 Worker 使用相同发布批次匹配两色镜像，但队列消费与 Service 切流相互独立。预发使用独立命名空间、独立队列/逻辑 DB 和测试凭据。生产候选清单可预置为零副本。只有已验证新旧消息协议、任务幂等与租约兼容时才允许重叠消费。否则先停止旧 Worker 接收新任务，等待处理中的任务确认或租约回收，然后启动候选、检查处理结果。脚本会并发执行同一命令中的所有角色。若本次版本对角色存在发布顺序依赖，发布人应拆成多条命令。分别记录各角色开始、就绪和耗时。

`account` 与未经验证可并行的 `scheduler` 采用先停旧再启新的交接。开始前记录旧副本数与版本。先将旧 Deployment 缩为零，按它的**实际 selector**确认包括 Terminating 在内的旧 Pod 已退出，并核对租约、会话或调度持有者已释放。再将新 Deployment 扩为一个副本。检查镜像、就绪状态和业务日志，确认只有一个活跃持有者。回滚按同样顺序先停新、确认退出及释放，再恢复旧实例。禁止强删失联 Pod 后立即启新，旧进程仍可能存活。
