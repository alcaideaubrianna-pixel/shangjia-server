# Publish observability metric contract

Review artifacts, 2026-11-18. Owned files: this document,
`deploy/observability/grafana/publish-chain-dashboard.json` and `publish-chain-alerts.yaml`.
Existing observability files are preserved. No deployment, online write, compile, test or commit
is part of this work. Dashboard JSON syntax validation is permitted.

## Verified ingestion and query scope

本轮只读查询验证：主助手通过 Grafana 工具执行了以下查询。
The main assistant obtained the following results through Grafana tools during this round:

```promql
count by (__name__) (
  {__name__=~"xiaohuiji[._]tg[._]api[._]requests.*|xiaohuiji[._]publish[._]jobs.*"}
)
```

Only `xiaohuiji_tg_api_requests_total` was returned, with 4 series; no dotted metric name or
publish-jobs series matched. This verifies the translated legacy API name only. The original
query matches plural jobs, not the final singular job.transitions/job.duration, so it does not
establish absence or availability of those final terminal instruments.

```promql
count by (job, deployment_environment_name, service_name, runtime_role, operation, upload_mode, result) (
  xiaohuiji_tg_api_requests_total
)
```

本轮只读查询验证的维度为 `job=youban`, `operation=sendMediaGroup`,
`upload_mode=auto|fallback`, `result=error|success`. No deployment_environment_name, service_name
or runtime_role was directly attached. No raw target_info values or secrets are reproduced here.

**Artifacts default to Prometheus translation, datasource UID `victoriametrics`, job="youban".**
There is no environment dropdown or invented resource filter/grouping. The optional instance
filter defaults to `.*`, which also matches a series without the instance label. Instance values
are queried from the implemented legacy API metric; other instances can be entered as custom
values if present on new instruments. No instance label was proved by this round’s query result.

For resource enrichment, inspect target_info labels and cardinality privately. Only then consider
an explicitly environment-scoped `on(job,instance) group_left(...)` join. Its RHS must be unique
for each match key; missing instance or duplicate targets across environments makes such a join
unsafe/ambiguous. Neither artifact performs that join. A single job value is not an environment
boundary: if the datasource contains multiple environments with job=youban, use an established
datasource/tenant boundary or a verified join before enabling alerts. Never infer an environment
from the absence of labels.

`server/internal/global/telemetry.go` exports OTLP/gRPC; the historical checked-in Collector
exports OTLP/HTTP to OpenObserve and does not establish current VictoriaMetrics flags. Existing
older dotted-name alert files are not evidence of current ingestion. Full Prometheus translation
adds type/unit suffixes; names below combine source instrument type/unit with the verified
translation convention. Only the legacy API name has been live verified in this session.

| Source OTel instrument | Artifact Prometheus series | Meaning / unit |
| --- | --- | --- |
| xiaohuiji.publish.job.transitions | xiaohuiji_publish_job_transitions_total | Changed committed terminal transitions; not distinct lifetime jobs |
| xiaohuiji.publish.job.duration | xiaohuiji_publish_job_duration_seconds_{bucket,sum,count} | Creation to terminal commit, seconds; source-confirmed singular job.duration |
| xiaohuiji.publish.projection.commits | xiaohuiji_publish_projection_commits_total | Changed committed terminal-confirmation projections |
| xiaohuiji.publish.projection.duration | xiaohuiji_publish_projection_duration_seconds_{bucket,sum,count} | Committed changed projection operation duration, seconds |
| xiaohuiji.publish.api.requests | xiaohuiji_publish_api_requests_total | Concrete completed client calls |
| xiaohuiji.publish.api.duration | xiaohuiji_publish_api_duration_seconds_{bucket,sum,count} | Concrete client-call duration, seconds |
| xiaohuiji.publish.fallback.events | xiaohuiji_publish_fallback_events_total | Branch/handoff lifecycle observations |
| xiaohuiji.publish.cleanup.message.requests | xiaohuiji_publish_cleanup_message_requests_total | Concrete Bot deleteMessage invocations, not unique messages |
| xiaohuiji.publish.cleanup.message.duration | xiaohuiji_publish_cleanup_message_duration_seconds_{bucket,sum,count} | One concrete Bot deleteMessage call duration, seconds |
| xiaohuiji.publish.recovery.steps | xiaohuiji_publish_recovery_steps_total | Recovery invocations |
| xiaohuiji.publish.recovery.duration | xiaohuiji_publish_recovery_duration_seconds_{bucket,sum,count} | Recovery invocation duration, seconds |
| xiaohuiji.publish.recovery.items | xiaohuiji_publish_recovery_items_total | Per-item recovery attempts |
| xiaohuiji.publish.recovery.page_items | xiaohuiji_publish_recovery_page_items | Last bounded page size, gauge |
| xiaohuiji.publish.recovery.page_oldest_age | xiaohuiji_publish_recovery_page_oldest_age_seconds | Oldest unchanged age in queried page, gauge seconds |
| xiaohuiji.publish.observation.updated_at | xiaohuiji_publish_observation_updated_at_seconds | Successful scan Unix timestamp, gauge seconds |
| xiaohuiji.publish.runtime.last_seen | xiaohuiji_publish_runtime_last_seen_seconds | Observed runtime Unix timestamp, gauge seconds |
| xiaohuiji.publish.runtime.heartbeats | xiaohuiji_publish_runtime_heartbeats_total | Runtime heartbeat observations |
| xiaohuiji.tg.api.requests | xiaohuiji_tg_api_requests_total | Selected legacy API attempts; live name verified |
| xiaohuiji.tg.api.duration_seconds | xiaohuiji_tg_api_duration_seconds_{bucket,sum,count} | Legacy API attempt duration, seconds |
| xiaohuiji.tg.media_send_requests | xiaohuiji_tg_media_send_requests_total | Legacy aggregate phase sends |
| xiaohuiji.tg.media_send_duration_seconds | xiaohuiji_tg_media_send_duration_seconds_{bucket,sum,count} | Legacy aggregate phase duration, seconds |
| xiaohuiji.tg.profile_publish_total | xiaohuiji_tg_profile_publish_total | Legacy execution observations; no duplicate _total suffix |
| xiaohuiji.tg.profile_publish_queue_wait_seconds | xiaohuiji_tg_profile_publish_queue_wait_seconds_{bucket,sum,count} | Creation to each send execution entry, retry inclusive |
| xiaohuiji.tg.delete_fallback_events | xiaohuiji_tg_delete_fallback_events_total | Delete fallback lifecycle observations |

The historical SQL receiver uses `xiaohuiji.publish.jobs` as an inventory gauge. It retains its
name. The source-confirmed terminal counter is **xiaohuiji.publish.job.transitions**; do not rename the historical inventory or query it as a terminal counter. There is
no union of native and translated names and no addition of both ingestion representations.

## Terminal job and projection contract

Source: `publish_terminal_observability.go` and postcommit owners in `tg_job_lifecycle.go`
and `tg_send_job.go`; transaction helpers are in `tg_job_transition.go`. Direct source inspection
confirms singular **xiaohuiji.publish.job.transitions** and **xiaohuiji.publish.job.duration**,
both with status=sent/failed/superseded. The sent hook runs in shared completeTelegramJob only
after the owning confirmation transaction has returned successfully and changed=true. It covers
Bot/account/relay flows that use this shared completion path. The superseded lifecycle hook is
also wired. handleTelegramJobError now emits only after an actual changed committed transition;
the metric helper filters out retry/nonterminal statuses, so typical send-path terminal failures
are observed. Watchdog and batch terminal paths are not all instrumented; no full success-rate
SLO, complete failure count or complete terminal-status census is claimed.
The helper checks top-level transaction ownership; ambient transactions do not assert an outer
commit. Terminal panels show recorded transition observations and their duration, never profiles
or unique lifetime jobs. No terminal success/failure ratio alert is provisioned.

| Object | Start | End / count boundary | Exact metric labels / retry semantics |
| --- | --- | --- | --- |
| Job terminal transition | Durable job admission defines job identity | After successful outer owning transaction returns, changed transition=true, no commit error, terminal status sent/failed/superseded | status only. No count for rollback, unchanged conditional update, defer, unknown, retry state or fallback acceptance. Explicit retry followed by another terminal transition counts again. |
| Job duration | Job created_at | Same changed terminal commit boundary | status; histogram seconds. Missing/invalid creation time skips duration but not transition count. |
| Terminal-confirmation projection | Begin confirmTelegramJobAndCyclePlan / confirmTelegramJobAndRecovery | Changed terminal-confirmation transaction returns after successful outer commit; ambient transaction contexts are excluded by the top-level guard | kind=terminal_confirmation. Commits count changed terminal-confirmation operations, not rows, messages, profile inventory or eventual asynchronous profile/index projection. Duration seconds uses the same changed successful boundary. |

An execution returning nil does not establish job success. Terminal status must come from the
winning durable transition. A flag set inside a transaction callback is insufficient until the
outer transaction commit succeeds. Nested transaction ownership requires explicit inspection.
A postcommit in-memory counter can lose a point if the process crashes after commit; replayed
callbacks can duplicate points unless guarded. These are operational metrics, not durable audit
totals. A durable outbox or database reconciliation is required for audit-grade counts.

## Concrete API and fallback contract

Source: `publish_transport_observability.go`, `tg_send_media.go`, and
`tg_media_fallback_handoff.go`. No entity IDs enter the new namespace.

| Instrument | Exact dimensions |
| --- | --- |
| api.requests | method, purpose, transport, outcome, error_class |
| api.duration | method, purpose, transport, upload_mode, outcome |
| fallback.events | purpose, transport, upload_mode, outcome, error_class, event |

method is bounded to sendMessage/sendPhoto/sendVideo/sendMediaGroup/copyMessage/copyMessages/other.
purpose is display/verify/mixed/unknown; transport bot/account/unknown; upload_mode is
file_id/url/binary/copy/mixed/not_applicable. outcome is success/failed/timeout/canceled/unknown.
error_class is none/unknown/ambiguous/timeout/canceled/rate_limited/permission/rejected/other.
Fallback event enums are code-defined and must include each emitted branch; unmatched values map
to other, so inspect final allowlist when adding branches. No raw method or error text is a label.

Start immediately before a concrete transport invocation; end once it returns. Count and duration
exclude preparation, wrapper retries and queue submissions. Every concrete retry/fallback call
is another attempt. A nil error without a confirmed expected response is unknown, not success.
Timeout/unknown does not prove Telegram failed to deliver. Request counters omit upload_mode;
histograms omit error_class. Compare them only after aggregating to shared dimensions. Histogram
bucket/sum/count carry identical dimensions and record seconds, not milliseconds.

Inspected Bot call sites include sendMessage/sendMediaGroup/sendPhoto/sendVideo/copyMessage/
copyMessages, relay calls and concrete fallback calls. Account handoff lifecycle events are not account API
requests. Separate high-level account phase observations are now implemented below; they do not
measure underlying account RPCs. Confirm actual account RPC instrumentation separately before
claiming complete transport coverage. Branch event/queued/claimed/completed observations may overlap for one task; summing all
events never produces distinct completed jobs or messages. Accepted/delayed/rejected handoffs
remain unknown until a phase result is known.

For event=recovered, reason=missing_or_terminal_account_task records outcome=unknown and
error_class=unknown: restoring the recovery path is not proof of successful delivery. In contrast,
reason=confirmed_facts_restored retains outcome=success because confirmed phase facts were
restored. Never infer success from event=recovered alone. reason is available in structured
handoff logs, not a fallback metric label; use outcome/error_class on metrics and correlate
jobId/traceId to logs when the recovery reason is needed.

## Recovery and runtime contract

Source: `publish_recovery_observability.go`, `tg_job_recovery.go`, and `observe_metrics.go`.
These factories and hooks are implemented in the working tree; live availability of their
translated series depends on the running build emitting observations.

| Instrument | Exact dimensions and boundary |
| --- | --- |
| recovery.steps / recovery.duration | step,outcome; one bounded invocation; success/failed/timeout/canceled. Duration explicit buckets .005,.01,.05,.1,.5,1,5,10,30,60 seconds. Step names are allowlisted. |
| recovery.items | step=confirmed_job, phase=repair_verify_purpose/complete_job, outcome=success/failed/timeout/canceled. Per-item attempts; retries count again. |
| recovery.page_items | kind=confirmed_job_page. Last successfully queried bounded page size, including zero. NOT global backlog. |
| recovery.page_oldest_age | kind=confirmed_job_page. Float gauge seconds: maximum now-updated_at in the ordered queried page; zero for empty/no dated rows. NOT histogram/global backlog age. |
| observation.updated_at | kind=confirmed_job_page. Float gauge Unix seconds updated only after successful scan, including an empty scan; failure leaves timestamp unchanged. |
| runtime.last_seen / runtime.heartbeats | role=account/scheduler/push-worker/media-worker/background-worker. Existing 30s loop; last_seen is Unix seconds, heartbeats counts observations, neither proves queue progress. |

The confirmed query selects sending/confirming jobs whose phases are verify_confirmed or
completed_no_verify and updated_at is at least 30s old; it orders by updated_at then id and limits
the page. It records the page before repairs. Item errors can be logged and continued, so
**step success != repaired success**. complete_job counts completion attempts, not unique repairs.

Page panels require scan age between 0 and 180s, matching job,instance,kind; replicas are not
summed as backlog. Repair-failure alerts require a fresh scan on job,instance. These joins do not
invent environment/service labels; if instance is absent or distinct producers collapse onto the
same label set, the data cannot establish per-replica freshness. Verify producer identity before
using that interpretation. Independent step-failure and stale-sample rules still reveal scan
failures without a fresh-page gate. Cached gauge reexports do not reset its embedded timestamp.

Publish no longer writes constant 1 into the global runtime heartbeat timestamp gauge. Its new
last_seen metric removes that collision. Stale-heartbeat alerting covers observed series only;
missing expected instances/roles require configured inventory. A healthy global sum cannot prove
all expected roles exist or all channels progress.

## Dashboard and alert behavior

The dashboard uses Prometheus-translated selectors and job=youban, with an optional instance
filter. Display is the default visible media phase. Legacy media panels use phase; new transport
panels map that variable to purpose. Selecting All includes mixed/unknown purposes as well.
Recovery phase is a different repair-phase namespace and is explicitly independent of the media
filter. Legacy API/execution metrics have no phase label. The legacy API operation dropdown
applies only to legacy API panels; new transport panels retain method in their breakdown.

Count gates require >=20 observations in 5m for P95/error fraction. P95 buckets aggregate by le
and the displayed phase/purpose. Fractions are 0..1; duration is seconds; rates are observations
per second; selected-range counts use increase. Apply rate/increase before replica aggregation.
No or vector(0), zero-fill or connected null spans: missing telemetry is not green success.
Only existing instrument observations appear; idle event-only series may be absent at startup.

`publish-chain-alerts.yaml` is Grafana unified alerting file provisioning, NOT a vmalert rules
file. orgId=1, folder Publish chain, datasource UID victoriametrics. Ten rules are paused for
review: legacy API error fraction and P95, delete fallback failures, recovery step failures,
fresh-scan item failures, stale confirmed-page sample, old work in a fresh page, observed runtime
heartbeat age, terminal-confirmation failed/timeout attempts, and cleanup failed/timeout attempts. Query A measures the value, B reduces strictly, C compares the threshold. NoData
and execution Error remain visible states; no notification policy/contact point is changed.

Review thresholds: API error fraction >20% with >=20 calls/5m; API P95 >10s with >=20 observations;
>=3 failure events/10m; page scan age >180s; old sampled work >600s with fresh nonempty page;
heartbeat age >120s. Confirm collection cadence, clock synchronization and notification routing.
Rules have fixed job=youban scope independent of dashboard variables. API rules aggregate the
observed target fleet; they have no verified environment split. They do not claim complete job
success SLO, global backlog or stalled-channel coverage.

## Trace/log correlation and label compatibility

`tg_publish_trace.go` supplies jobId, operationNo, executionId, taskId, queueRetryCount, traceId
and business identity in structured logs. Join attempts by jobId+operationNo and distinguish retries
with executionId/taskId. A trace ID can be reused across attempts. Carry traceparent/tracestate
through queue and account handoff; when changing traces retain causal identity and a span link.
Propagation coverage must be reviewed at each handoff rather than assumed from instrument presence.

Use telegram_publish_stage stageId/stage/event/durationMs/deadline/context fields and
telegram_publish_summary executionDurationMs/returnedError. A nil returned error can mean defer,
supersede or fallback queued. Nested stage times cannot be summed as wall time. Detailed logs
sample executions; OTel traces are sampled too, so neither supplies metric denominators. New
transport diagnostics add publish.api.request span events and publish_api_request warning logs
with normalized dimensions and durationSeconds/traceId for slow/non-success calls. New structured
logs standardize traceId,jobId,attemptId,durationSeconds where the corresponding identity is
available; retain executionId/taskId to distinguish executions. Span-event attributes can still
use trace_id/duration_seconds. Cleanup retains legacy traceID and durationMs for compatibility;
log queries must select the correct field spelling rather than assume a silent historical rename.

Main removes profile_id/profile_no from legacy publish/queue-wait/delete/media-send metrics and
misleading channel_name=TargetChatId from media-send. channel_id remains for compatibility only;
new namespace metrics have no IDs. Filters on removed labels stop receiving new points. Historical
series retain them until retention expires; rate/increase before aggregation handles rollout
series splits. Use logs/traces for profile lookup. The legacy source mapper now maps unpublish:
to web; historical source=auto counts are not backfilled. No logs/traces datasource UID or fake
links are invented; configure real Explore correlations separately. Redact tokens and signed URLs.

## Legacy audit and deferred inventory/stall monitoring

1. profile_publish_total counts send execution before completeTelegramJob and fallback finalization;
   legacy success/duration semantics remain unchanged and are deprecated for business-success SLOs.
2. Queue-wait is creation-to-send-entry on every retry, including prior work/backoff, not queue residence.
3. Legacy API instrumentation covers selected media-group calls; aggregate phase sends are not
   individual API calls or messages. The new concrete-call contract must not inherit these meanings.
4. Legacy delete fallback event/message observations overlap lifecycle stages; queued+success does not
   equal distinct messages. Legacy publish_failures observations do not prove exactly-once commits.
5. Historical dashboards use or vector(0) and historical dotted rules do not match the now verified
   translated API series. Neither defect is silently copied into these new artifacts.
6. SQL rolling-window inventory is a snapshot, not a counter to rate/increase. Its jobs gauge retains
   its name; terminal job.transitions avoids the type/ownership collision.
7. Resource attributes created in application code are not proof of directly promoted business labels.
   Actual observed labels govern query scope; target_info enrichment requires verified uniqueness.

Proposed only; no executable rules or fabricated panels until authoritative gauges exist:

| Proposed gauge | Required source/behavior | Future alert condition |
| --- | --- | --- |
| publish.inventory.jobs | Authoritative DB eligible status counts, explicit zeros, single collection owner | Reviewed capacity threshold, fresh successful snapshot |
| publish.inventory.oldest_eligible_age_seconds | DB eligibility excludes future schedules/intentional waits | Age plus nonzero eligible backlog and freshness |
| publish.channel.eligible_jobs / oldest_eligible_age_seconds | Per-channel inventory with explicit cardinality budget | Affected channel stays visible despite other channels' throughput |
| publish.channel.last_progress_timestamp_seconds | Committed progress per expected channel | No progress with eligible backlog, excluding cooldown/lease/rate-limit waits |
| publish.inventory.snapshot_timestamp_seconds / collection_success | Freshness including zero-row snapshots | Monitor collector failure separately from business stall |
| publish.projection.pending / oldest_pending_age_seconds | Durable outbox/projection inventory | Projection lag with fresh authoritative backlog |

Specify expected channels/roles, deletion/TTL semantics and collection ownership. Never sum
identical snapshots from replicas. A fresh bounded recovery page provides evidence about that
page only; it does not replace this inventory or establish complete stalled-channel coverage.

## Stage and terminal-confirmation attempt observations

`publish_trace_carrier.go` implements xiaohuiji.publish.stage.attempts (counter) and
xiaohuiji.publish.stage.duration_seconds (histogram seconds), both labels stage,result.
Prometheus names: xiaohuiji_publish_stage_attempts_total and
xiaohuiji_publish_stage_duration_seconds_{bucket,sum,count}. Results are returned/error/canceled/
deadline_exceeded/duplicate/conflict/retry/panic/abandoned. Unknown stages collapse to unknown. One point is
recorded per finished invocation, including retries; returned != business success. Histogram
boundaries are .001,.005,.01,.025,.05,.1,.25,.5,1,2.5,5,10,30,60,120 seconds. Consumer context
restoration and the real SendTelegramJob execution parent are wired by main; stage metrics are
unsampled observations, distinct from sampled traces/logs.

Each synchronous SendTelegramJob invocation owns a diagnostic stage registry
(`publish_stage_registry.go`). Normal stage completion removes its entry. On panic or early exit,
the registry closes forgotten inner stages and stops their diagnostic timers; it does not cancel
business work. Every stage still open in the registry is classified abandoned, regardless of
the enclosing execution error or panic; that cause does not replace the unfinished-stage verdict.
A directly deferred stage that reaches its own Finish during panic can report panic separately.
The finish guards prevent duplicate observations; abandoned is diagnostic incompleteness, not a
new persisted job status or evidence of a separately failed business operation.

The execution span records execution.result using the bounded result classifier, including
retry/duplicate/conflict. Those control-flow results do not mark the execution span Error;
error/canceled/deadline_exceeded do. A returned execution result still does not imply terminal
business success. Stage spans deliberately remain siblings under the active execution parent:
the helper starts a span but does not return a child context for subsequent stage calls. Timing
intervals may nest or overlap without a corresponding nested stage-parent tree. Do not infer
hierarchical stage parentage from elapsed times or sum stage durations as wall time.

Consumer context restoration and a real execution parent do not close every distributed-trace
gap. The separate unified-stream producer→consumer parent propagation gap remains; these changes
do not establish an unbroken end-to-end trace or resolve that handoff's causal parentage. No new
alert is added for abandoned or the execution control-flow classifications.

The single transaction-attempt family is `xiaohuiji.publish.projection.attempts` plus
`xiaohuiji.publish.projection.attempt.duration` (seconds). Prometheus names are
xiaohuiji_publish_projection_attempts_total and
xiaohuiji_publish_projection_attempt_duration_seconds_{bucket,sum,count}. Exact labels are
kind=terminal_confirmation, outcome=success/failed/timeout/canceled/noop, and
error_class=none/commit_error/timeout/canceled. Explicit duration boundaries are
.01,.05,.1,.5,1,5,10,30,60,120,300,600,900,1800,3600,7200 seconds.

Main calls observePublishTerminalProjection immediately after confirmTelegramJobAndCyclePlan or
confirmTelegramJobAndRecovery returns, then observePublishTerminalCommit after a real changed sent
transition. The top-level guard excludes ambient transactions. Attempts include noops/errors and
retries. Only success changed adds projection.commits/duration. This is the terminal-confirmation
plus channel-plan transaction, NOT later asynchronous profile/index projection completion. The
paused transaction failure rule requires >=3 failed/timeout attempts in 10m for 2m; canceled is
shown on panels but excluded from this failure rule. No duplicate transaction-attempt family is
specified or queried.

Transport API duration has explicit boundaries .005,.01,.05,.1,.5,1,5,10,30,60,120,300,600,1200
seconds. Terminal job duration and changed-commit projection duration have the same boundaries
as the projection attempt histogram above. Review P95 results against these bucket resolutions.

## Cleanup execution contract

Directly inspected `publish_cleanup_observability.go` and `tg_delete_job.go`. The implemented
instruments are xiaohuiji.publish.cleanup.attempts (counter) and
xiaohuiji.publish.cleanup.duration (histogram, seconds). Translated names are
xiaohuiji_publish_cleanup_attempts_total and
xiaohuiji_publish_cleanup_duration_seconds_{bucket,sum,count}.

Attempts carry reason,outcome,error_type; duration carries those labels plus stage. reason is
unpublish/delete/republish/edit/cancel/unspecified/other and derives from CleanupOperationNo,
not the historical publishing Job.OperationNo. error_type is none/canceled/timeout/retry_after/other.
Attempt outcomes are settled_attempt/deferred/incomplete/failed/timeout/canceled. Stage outcomes
can additionally be completed/skipped. Exactly one terminal attempt observation is recorded per
invocation by an atomic finish guard. Retries count again. A nil return or requeue is not proof
of cleanup settlement, and settled_attempt does not count distinct cleanup intents or profiles.

Stage allowlist: attempt/hydrate_payload/load_job/channel_cleanup_slot/job_cleanup_lease/
delete_messages/notify_unpublish_completed/finish_profile_state/settle_profile_cleanup_progress/
complete_cleanup_intent/requeue; unknown stages become other. Histogram boundaries are
.001,.005,.01,.025,.05,.1,.25,.5,1,2.5,5,10,30,60,120,300 seconds. Durations include nested stages;
end-to-end cleanup panels explicitly select stage=attempt, never sum all stages. Media phase
filters do not apply. The failure rule requires >=3 failed/timeout attempt observations in 10m
for 2m; deferred and incomplete are separately visible, not silently counted as success.

Cleanup spans use publish.cleanup.attempt and child stage names; structured logs use
telegram_cleanup / telegram_cleanup_stage with jobId, cleanupOutboxId, cleanupOperationNo,
traceID and durationMs. Identity is confined to redacted logs/spans, not metric dimensions.
No test or compilation was run for this artifact integration; reading test/source files does
not claim execution or production availability of the new metrics.

## Account phase sends and Bot failover

Directly inspected `publish_transport_observability.go`, `tg_media_fallback_handoff.go` and
`tg_send_job.go`. The existing high-level protocol-send result hook records
xiaohuiji.publish.account.phase.attempts (counter; purpose,outcome,error_class) and
xiaohuiji.publish.account.phase.duration (histogram seconds; purpose,outcome). Prometheus names:
xiaohuiji_publish_account_phase_attempts_total and
xiaohuiji_publish_account_phase_duration_seconds_{bucket,sum,count}. Duration boundaries are
.005,.01,.05,.1,.5,1,5,10,30,60,120,300,600,1200 seconds.

purpose/outcome/error_class use the same bounded enums as transport metrics. One observation is
one high-level phase-send invocation at its result hook; it may issue multiple underlying account
RPCs. Duration excludes task queue residence and is NOT per-RPC latency. Retries count again.
These panels remain separate from concrete Bot API request panels; do not combine their request
counts, latency populations or denominators. An account handoff event still is not a phase send
or an account API call.

Confirmation requires mediaCount >0 and all expected returned message IDs. A nil error with
empty/nil or incomplete IDs is unknown, not success. The existing SLS protocol result verdict is
also corrected so nil/empty replies do not appear successful. Neither a confirmed phase nor a
successful RPC independently proves the later terminal job/projection commit.

Bot rate-limit failover emits fallback.events with event=bot_switched,purpose=unknown,
transport=bot,upload_mode=not_applicable,outcome=unknown,error_class=rate_limited. This records the
branch decision, not another API request or a completed job. The dedicated Bot-switch panel is
independent of the media phase filter, so default purpose=display does not hide these events.
The general fallback panel still respects the purpose filter; select All/unknown to see failovers
there. Account-phase panels map the visible media phase selection to purpose and gate P95 at
>=20 duration observations per purpose in 5m. No new account-RPC alert or complete terminal SLO
is inferred from this addition.

## Concrete Bot message-delete calls

Directly inspected the messageRequest factory/helper in `publish_cleanup_observability.go` and
call sites immediately around bot.DeleteMessage in `telegram_message_set.go` and partial-send
rollback in `tg_send_job.go`. These emit xiaohuiji.publish.cleanup.message.requests (counter)
and xiaohuiji.publish.cleanup.message.duration (histogram, unit s). Both have exactly
transport=bot and outcome=deleted/already_absent/unknown/failed/timeout/canceled/deferred.
Prometheus names are xiaohuiji_publish_cleanup_message_requests_total and
xiaohuiji_publish_cleanup_message_duration_seconds_{bucket,sum,count}. Explicit boundaries are
.005,.01,.05,.1,.5,1,5,10,30,60,120 seconds.

Start immediately before each actual DeleteMessage invocation and record once on return.
true,nil means deleted; false,nil means unknown. An already-deleted/not-found error recognized
by the idempotency classifier becomes already_absent, separate from a newly confirmed deletion.
Other errors follow cleanup normalization: timeout/canceled/deferred (retry_after) or failed.
Retries and repeated deletion of the same message each produce another invocation observation;
request counts are NOT distinct messages, newly deleted message totals, settled cleanup intents
or profile counts. No success-rate SLO is inferred from deleted+already_absent.

Concrete-delete panels are separate from cleanup invocation and nested-stage panels. The outcome
rate shows every recorded outcome; the duration P95 uses matching buckets/count, grouped by
outcome, gated at >=20 observations/outcome in 5m. Units are calls/sec and seconds. The media phase
filter does not apply, since these metrics do not have purpose/phase. Protocol delete RPCs still
have existing fallback-event/stage coverage only; do not label these Bot-only call panels as
complete protocol or all-transport deletion monitoring. Existing ten paused alert rules are
unchanged; no unverified deletion inventory or stalled-cleanup rule is added.

New test sources were added by the implementing agents but were not run as part of this artifact
work. Source inspection and dashboard JSON syntax validation do not claim those tests passed.
