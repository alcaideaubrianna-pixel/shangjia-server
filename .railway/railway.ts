import {
  defineRailway,
  image,
  postgres,
  project,
  redis as railwayRedis,
  service,
} from "railway/iac";

const imageRef = "ghcr.io/mjiadfwaff-bot/youban-server:main";
const singapore = "asia-southeast1-eqsg3a";

export default defineRailway(() => {
  const database = postgres("Postgres");
  const cache = railwayRedis("Redis");
  const appImage = image(imageRef);

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
  };

  const api = service("youban-api", {
    source: appImage,
    start: "/app/hotgo web",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "web",
    },
  });

  const worker = service("youban-worker", {
    source: appImage,
    start: "/app/hotgo worker",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "worker",
    },
  });

  const runtime = service("youban-runtime", {
    source: appImage,
    start: "/app/hotgo runtime",
    healthcheck: "/readyz",
    replicas: { [singapore]: 1 },
    env: {
      ...commonEnv,
      YOUBAN_RUNTIME_ROLES: "runtime",
    },
  });

  return project("youban-production", {
    resources: [database, cache, api, worker, runtime],
  });
});
