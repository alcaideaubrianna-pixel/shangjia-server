# Alibaba Cloud SAE deployment

## Local deployment

Use the checked-in script for production deployments; do not hand-edit SAE
application settings between roles:

```bash
./deploy/sae-deploy-all.sh e5b746c
./deploy/sae-deploy-all.sh e5b746c web publish
```

The script owns the role command arguments, image, ACR instance, environment
preservation, one-at-a-time ordering, health polling, and Account singleton
constraints. Web is started with the `runtime` command so the Telegram Gateway
interactive and bulk consumers are actually started. Account is always last.

Do not change `CommandArgs` for Web to `web`: that starts HTTP routes but does
not start the Gateway Runtime queues. This exact mismatch caused the
2026-10-28 incident: Telegram requests returned HTTP 200 while interactive
Asynq tasks accumulated with no active consumer.

The GitHub Actions workflow builds one immutable image per commit. SAE Web
deployment is opt-in and runs only after the image build succeeds.

Configure these repository variables:

- `SAE_AUTO_DEPLOY=true`
- `SAE_REGION_ID=ap-southeast-1`
- `SAE_WEB_APP_ID=<SAE Web application id>`
- `SAE_WEB_HEALTH_URL=https://<web-domain>/healthz`

Configure these repository secrets:

- `ALIBABA_CLOUD_ACCESS_KEY_ID`
- `ALIBABA_CLOUD_ACCESS_KEY_SECRET`

The RAM principal should be limited to the SAE application used by this
workflow and have only the permissions needed to deploy and inspect it
(`sae:DeployApplication`, `sae:DescribeApplication`, and the corresponding
application update permission if the platform requires it).

The workflow deploys the SHA-tagged image, never the mutable branch or
`latest` tag, and waits for the configured health URL. Worker applications are
not included yet: enabling them before the PostgreSQL/Redis cutover is
verified can start a second queue consumer and duplicate Telegram deliveries.
Add separate, ordered deployment jobs only after each worker has its final
role command and production dependency configuration.

If `SAE_AUTO_DEPLOY` is absent or not exactly `true`, the workflow only builds
and pushes images; it does not change SAE state.


### Web 高可用部署

部署脚本对 `web` 自动使用 2 个实例、`MaxSurgeInstances=1`、`MinReadyInstances=1`，并等待两个实例均为健康状态；这样新版本启动失败时仍保留旧实例提供服务。其他 Worker 仍按单实例部署，`account` 保持单实例。


All production SAE changes must go through `deploy/sae-deploy-all.sh`; do not call `DeployApplication` manually. The script preserves ENV, injects media fallback and publish concurrency, keeps Web at two replicas, uses rolling readiness for workers, and keeps Account as a singleton.
