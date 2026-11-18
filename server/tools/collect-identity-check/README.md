# Collection identity regression binary

Build with `python3 tools/collect-identity-check/build.py` from `server/`.
This runs `go build`, never `go test`. The generated entry point invokes the
repository's regression cases directly using Go's testing assertion runner.
It does not import application bootstrap, start consumers or send TG messages.

Run the binary against a disposable PostgreSQL instance listening only on
loopback (user `test`, database `postgres`, trust authentication):

```sh
YOUBAN_MULTISOURCE_TEST_PORT=55497 /tmp/youban-collect-identity-check -test.v -test.timeout=90s
```

Every run creates and drops its own schema. No production DSN is read by these
cases. Coverage: M19327 repeated-display adjacency, T40073 late verification,
second independent verification rejection, same-album continuation, concurrent
identity writes, immutable target conflicts, rule/source isolation, 12 concurrent
verification commits preserving original display and projecting verification,
and four independent worker binary processes writing one identity.

# Deployment

Apply `addons/youban_publish/resource/sql/collect_identity.pgsql.sql` before
rolling out collector/media. No automatic DDL or historical sweep is added.
Historical duplicates without an identity cannot be reconstructed from error
text automatically. Reconcile them only from explicit audited branch evidence.
The collector merges material but does not republish already-sent TG messages.
Publish changes are not required for this migration.
