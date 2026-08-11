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
  const collectorData = volume("xiaohuiji-otel-data", {
    region: singapore,
    sizeMB: 1024,
  });
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
    start: "--config=/etc/otelcol-contrib/config.yaml",
    healthcheck: "/",
    replicas: { [singapore]: 1 },
    volumeMounts: {
      "/var/lib/otelcol": collectorData,
    },
    env: {
      XIAOHUIJI_PG_DSN: database.env.DATABASE_URL,
      OPENOBSERVE_OTLP_ENDPOINT: `http://${observe.env.RAILWAY_PRIVATE_DOMAIN}:5080/api/default`,
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
      YOUBAN_TCP_CRON_ADDRESS: "xiaohuiji-api.railway.internal:8099",
    },
  });

  return project("xiaohuiji-production", {
    resources: [database, cache, observeData, collectorData, observe, collector, api, worker, account, scheduler],
  });
});
