# Alibaba Cloud SAE deployment

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
