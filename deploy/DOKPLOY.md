# Dokploy deployment

The production server image is built once from
`feat/railway-runtime-ci-split` and published to GHCR with two tags:

- `feat-railway-runtime-ci-split`: the fixed tag configured in Dokploy
- `sha-<short-commit>`: an immutable audit and rollback tag

Deployment targets are maintained in `deploy/dokploy-targets.json`. Targets
are called sequentially by ascending `order`. A target is skipped unless its
`enabled` value is exactly `true`.

## Initial configuration

For every Dokploy application:

1. Set the Docker image to
   `ghcr.io/alcaideaubrianna-pixel/youban-server:feat-railway-runtime-ci-split`.
2. Configure GHCR credentials that can pull this private package.
3. Copy the application deploy webhook into `deploy/dokploy-targets.json`.
4. Set `enabled` to `true` only after that runtime has completed cutover.

The webhook URL contains a deployment token. This project intentionally keeps
these URLs in `deploy/dokploy-targets.json` for centralized maintenance. The
file is excluded from the Docker build context and the repository must remain
private.

GitHub Actions triggers targets by ascending `order`. After triggering a
target, it waits for `waitSeconds`; when `healthUrl` is configured, deployment
continues only after the endpoint returns HTTP 2xx. When `verifyRevision` is
enabled, CI also requires consecutive responses from the requested Git
revision. This prevents one healthy new replica from hiding an old replica
during a rolling update. Put the API first so a failed rolling update stops
deployment before singleton services and workers.

## Migration order

Enable and verify one target at a time:

1. `xiaohuiji-api`: start on a temporary domain and verify `/readyz`; it may
   run alongside Railway during HTTP traffic testing.
2. `xiaohuiji-collector-worker`: stop its Railway service before enabling it.
3. `xiaohuiji-media-worker`: stop its Railway service before enabling it.
4. `xiaohuiji-worker`: stop its Railway service before enabling it.
5. `xiaohuiji-publish-worker`: stop its Railway service before enabling it.
6. `xiaohuiji-account`: stop its Railway service before enabling it.
7. `xiaohuiji-scheduler`: migrate last and never run it in both environments.

Workers, account runtimes, and the scheduler consume shared Redis state or own
singleton work. Do not enable their Dokploy targets while the matching Railway
service is still running.

After each cutover, check application health, database/Redis connections,
queue backlog, error logs, and Telegram delivery before continuing. Move API
traffic gradually at the edge only after the new API runtime is healthy.

To disable automatic deployment without changing Dokploy, set the target's
`enabled` value back to `false`. To roll back, configure the application to a
known-good `sha-<short-commit>` image and redeploy it manually.
