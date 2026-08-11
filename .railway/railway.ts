import {
  defineRailway,
  image,
  postgres,
  preserve,
  project,
  redis as railwayRedis,
  service,
  volume,
} from "railway/iac";

const imageRef = process.env.YOUBAN_RAILWAY_IMAGE?.trim();
const collectorImageRef = process.env.XIAOHUIJI_OTEL_COLLECTOR_IMAGE?.trim();
const singapore = "asia-southeast1-eqsg3a";

if (!imageRef) {
  throw new Error(
    "YOUBAN_RAILWAY_IMAGE is required, for example ghcr.io/<owner>/youban-server:sha-abc1234",
  );
}

if (!collectorImageRef) {
  throw new Error(
    "XIAOHUIJI_OTEL_COLLECTOR_IMAGE is required, for example ghcr.io/<owner>/xiaohuiji-otel-collector:sha-abc1234",
  );
}

export default defineRailway(() => {
  const database = postgres("Postgres");
  const cache = railwayRedis("Redis");
  const appImage = image(imageRef);
  const collectorImage = image(collectorImageRef);
  const openObserveImage = image("openobserve/openobserve:v0.92.0");
  const observeData = volume("xiaohuiji-observe-data", {
    region: singapore,
    sizeMB: 5000,
  });

  const commonEnv = {
    YOUBAN_APP_NAME: "youban",
    YOUBAN_MODE: "product",
    YOUBAN_DEBUG: "false",
    YOUBAN_DB_DRIVER: "pgsql",
    YOUBAN_DB_HOST: database.env.PGHOST,
    YOUBAN_DB_PORT: database.env.PGPORT,
    YOUBAN_DB_NAME: database.env.PGDATABASE,
    YOUBAN_DB_USER: database.env.PGUSER,
    YOUBAN_DB_PASSWORD: database.env.PGPASSWORD,
    YOUBAN_DB_PREFIX: "hg_",
    YOUBAN_DB_DEBUG: "false",
    YOUBAN_REDIS_HOST: cache.env.REDISHOST,
    YOUBAN_REDIS_PORT: cache.env.REDISPORT,
    YOUBAN_REDIS_PASSWORD: cache.env.REDISPASSWORD,
    YOUBAN_REDIS_DB: "0",
    YOUBAN_CACHE_ADAPTER: "redis",
    YOUBAN_QUEUE_DRIVER: "redis",
    YOUBAN_TELEMETRY_SWITCH: "true",
    YOUBAN_TELEMETRY_ENDPOINT: "xiaohuiji-otel-collector.railway.internal:4317",
    YOUBAN_TELEMETRY_SECURE: "false",
    YOUBAN_TELEMETRY_SAMPLE_RATIO: "0.15",
    YOUBAN_TELEMETRY_METRICS_INTERVAL_SECONDS: "15",
    YOUBAN_TELEGRAM_COLLECTOR_ENABLED: "true",
    YOUBAN_TELEGRAM_COLLECTOR_CONCURRENCY: "12",
    YOUBAN_TELEGRAM_COLLECTOR_RECOVERY_BATCH_SIZE: "200",
    YOUBAN_TELEGRAM_DELIVERY_CONCURRENCY: "8",
    YOUBAN_TELEGRAM_MEDIA_CONCURRENCY: "6",
    YOUBAN_TELEGRAM_ACCOUNT_LEASE_SECONDS: "45",
  };

  const observe = service("xiaohuiji-observe", {
    source: openObserveImage,
    healthcheck: "/healthz",
    replicas: { [singapore]: 1 },
    volumeMounts: {
      "/data": observeData,
    },
    env: {
      ZO_DATA_DIR: "/data",
      ZO_LOCAL_MODE: "true",
      ZO_HTTP_PORT: "5080",
      PORT: "5080",
      ZO_ROOT_USER_EMAIL: preserve(),
      ZO_ROOT_USER_PASSWORD: preserve(),
      ZO_TELEMETRY: "false",
    },
  });

  const collector = service("xiaohuiji-otel-collector", {
    source: collectorImage,
    start: "/otelcol-contrib --config=/etc/otelcol-contrib/config.yaml",
    healthcheck: "/",
    replicas: { [singapore]: 1 },
    env: {
      XIAOHUIJI_PG_DSN: database.env.DATABASE_URL,
      OPENOBSERVE_OTLP_ENDPOINT: "http://xiaohuiji-observe.railway.internal:5080/api/default",
      OPENOBSERVE_AUTHORIZATION: preserve(),
      PORT: "13133",
    },
  });

  const api = service("xiaohuiji-api", {
    source: appImage,
    start: "/app/hotgo web",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "web",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-api",
    },
  });

  const worker = service("xiaohuiji-worker", {
    source: appImage,
    start: "/app/hotgo worker",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "worker",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-worker",
    },
  });

  const collectorWorker = service("xiaohuiji-collector-worker", {
    source: appImage,
    start: "/app/hotgo collector-worker",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "collector-worker",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-collector-worker",
    },
  });

  const mediaWorker = service("xiaohuiji-media-worker", {
    source: appImage,
    start: "/app/hotgo media-worker",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "media-worker",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-media-worker",
    },
  });

  const publishWorker = service("xiaohuiji-publish-worker", {
    source: appImage,
    start: "/app/hotgo publish-worker",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "push-worker",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-publish-worker",
    },
  });

  const account = service("xiaohuiji-account", {
    source: appImage,
    start: "/app/hotgo account",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "account",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-account",
    },
  });

  const scheduler = service("xiaohuiji-scheduler", {
    source: appImage,
    start: "/app/hotgo scheduler",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "scheduler",
      YOUBAN_TELEMETRY_SERVICE_NAME: "xiaohuiji-scheduler",
      YOUBAN_TCP_CRON_ADDRESS: "xiaohuiji-api.railway.internal:8099",
    },
  });

  return project("xiaohuiji-production", {
    resources: [
      database,
      cache,
      observeData,
      observe,
      collector,
      api,
      worker,
      collectorWorker,
      mediaWorker,
      publishWorker,
      account,
      scheduler,
    ],
  });
});
