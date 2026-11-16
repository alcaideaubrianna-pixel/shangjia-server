# Collector Pipeline Architecture Contract

## Purpose

This document defines the ownership boundary between collection and delivery in the Youban pipeline.

The Collector is responsible for receiving source messages, persisting collection data, completing collection-side processing, and placing an eligible delivery record into the delivery queue. The Collector contract ends when the delivery record is durably marked as queued.

Telegram sending, target-channel permissions, Telegram API calls, send retries, send confirmation, and final publication status belong to the Delivery/Publish worker.

## Responsibility boundary

```text
Telegram source message
        |
        v
Collector ingress
        |
        v
Persist raw collection event
        |
        v
Persist normalized material and media references
        |
        v
Apply collection rules / approval decision
        |
        v
Create or update delivery record
        |
        v
Durably enqueue delivery work
        |
        v
[Collector responsibility ends]

Delivery/Publish worker
        |
        v
Claim delivery work
        |
        v
Resolve target channel and account
        |
        v
Call Telegram API
        |
        v
Persist send attempt and final publication result
```

The Collector must not wait for Telegram delivery completion before acknowledging collection work. A delivery task may remain pending, retrying, processing, or dead after the Collector has successfully handed it off.

## Ownership terminology

Use these terms consistently in code, logs, metrics, and incident reports:

| Term | Meaning | Owner |
|---|---|---|
| `collection event` | A source message received by the system | Collector |
| `material` | Normalized display or verification content derived from a collection event | Collector / Publish preparation |
| `collect processing` | Rule matching, material grouping, media readiness, and collection-side state transitions | Collector pipeline |
| `delivery record` | Durable unit of work describing material eligible for a target publication path | Collector creates; Delivery consumes |
| `dispatch queued` | Delivery record is durably eligible for Delivery worker consumption and has been handed to the delivery queue | Collector terminal state |
| `delivery claim` | Delivery worker changes a queued record to processing under a lease | Delivery worker |
| `send attempt` | One attempt to call Telegram or another publication provider | Delivery/Publish worker |
| `published` | Telegram/provider success has been confirmed and persisted | Delivery/Publish worker |
| `delivery retry` | A failed or uncertain send is scheduled for another Delivery attempt | Delivery/Publish worker |
| `dead delivery` | Delivery exceeded its retry policy or was permanently rejected | Delivery/Publish worker |

Do not describe a delivery as “published” when the Collector has only created a delivery record or queued a delivery task. Do not attribute Telegram permission failures or send retries to the Collector.

## Queue lanes

The queue system has independent logical lanes:

- realtime collection: newly received source work that should be processed promptly;
- history collection: explicitly historical or recovery work;
- delivery realtime: eligible delivery work from realtime collection;
- delivery history: eligible delivery work from historical or recovery collection;
- delivery retry/dead: Delivery worker recovery outcomes.

A queue name, shard, or Redis key is an execution detail. The durable database record remains the source of truth for business state. Queue depth alone does not prove that a record has been claimed or published.

Dynamic source or account queues may share physical shards for operational reasons. Logical fairness must still be enforced by bounded batches, per-source or per-account quotas, and independent realtime/history budgets.

## Collector completion contract

A collection flow may return success only after all of the following are true:

1. The raw collection event has been persisted or is safely deduplicated.
2. Collection-side material and media state is consistent with the event outcome.
3. Rule matching or approval processing has reached a durable collection-side state.
4. An eligible delivery record exists when publication is required.
5. The delivery record is durably marked `dispatch_queued` or an equivalent queued state.
6. The queue handoff is observable with tenant, account, source, event, delivery, lane, and queue identifiers.

The Collector must not mark a delivery as published, sent, Telegram-successful, or target-channel-complete.

If the queue handoff fails, the Collector must preserve a recoverable durable state and expose the failure for bounded recovery. It must not silently acknowledge a collection event while losing the delivery handoff.

## Delivery ownership contract

The Delivery/Publish worker owns all work after `dispatch_queued`:

- claiming the delivery record with a lease;
- resolving the target account and channel;
- checking target-channel readiness;
- invoking Telegram APIs;
- recording send attempts;
- handling retryable, uncertain, and permanent failures;
- confirming Telegram success;
- marking the delivery as published, retrying, skipped, or dead;
- reconciling uncertain Telegram outcomes.

A Delivery failure must not cause the Collector to re-collect the source message. Delivery recovery should requeue the delivery record through the Delivery lane while preserving the original collection event and material identity.

## Durable state transitions

The expected ownership transition is:

```text
collection_event: received
    -> processing
    -> processed / ignored / failed

 delivery_record: created
    -> dispatch_queued        Collector-owned terminal handoff state
    -> processing             Delivery claim
    -> published              Delivery success
    -> retry_wait             Retryable Delivery failure
    -> dead                   Permanent or exhausted Delivery failure
```

`dispatch_queued` means “ready for Delivery consumption.” It does not mean “Telegram accepted the message.”

The database-to-Redis handoff must be recoverable. If the database says `dispatch_queued` but Redis has no executable task, a Delivery recovery process must be able to reconstruct the task without re-running collection business logic.

If a worker claims a delivery and exits before completion, the lease must expire and the Delivery recovery process must make the record retryable. A stale queue task must not permanently block the durable record.

## Fairness and isolation requirements

The pipeline must satisfy these rules:

- Realtime collection work must have an independent concurrency budget from history recovery.
- A single tenant, account, or source must not consume an unbounded recovery batch.
- Recovery queries must be bounded and lane-specific.
- Recovery must use indexed predicates and stable ordering.
- A source with continuous arrivals must not starve older sources.
- Delivery worker capacity must be observable separately from Collector capacity.
- Queue depth and oldest-item age must be reported per lane and, where practical, per source or account.
- A delivery backlog must not cause the Collector to repeatedly fetch or reprocess the same source message.

The preferred scheduling model is a durable outbox with lease-based claiming, bounded `FOR UPDATE SKIP LOCKED` batches, per-lane budgets, and weighted or deficit round-robin fairness across active sources.

## Retry and permanent failure semantics

Collection and Delivery have different retry domains.

Collector retries cover failures before `dispatch_queued`, such as source ingestion, normalization, media preparation, rule processing, or durable delivery creation. They must preserve idempotency through event keys and delivery deduplication.

Delivery retries cover failures after `dispatch_queued`, such as Telegram rate limits, network failures, target-channel readiness, media send failures, and uncertain API results. They must not create duplicate collection events.

A retryable failure must retain enough state to be retried with a bounded backoff. A permanent failure must be observable and must not be retried forever. The final status must identify which owner made the decision.

## Required observability

Every handoff and recovery record should include, when available:

- `tenant_id`
- `account_id`
- `source_id`
- `source_chat_id`
- `source_message_id`
- `collect_event_id`
- `delivery_id`
- `scheduler_pool` or lane
- physical queue name or shard
- worker role
- worker instance and revision
- attempt count
- lease owner and lease expiry
- oldest pending age
- last error category

The minimum stage sequence for a delivery is:

```text
collection.delivery.created
collection.delivery.dispatch_queued
delivery.process.started
delivery.claimed
delivery.handler.started
delivery.handler.completed
delivery.completed
```

Failure stages should distinguish:

```text
collection.dispatch_failed
delivery.claim_failed
delivery.handler_failed
delivery.retry_scheduled
delivery.dead
delivery.completed_persist_failed
```

The absence of `delivery.process.started` after `dispatch_queued` indicates a queue or worker handoff problem. The presence of `delivery.process.started` without `delivery.claimed` indicates a claim or database problem. Telegram API and target-channel failures belong after the claim stage.

## Operational rule

When investigating a missing publication, follow the durable handoff in this order:

1. source message and collection event;
2. material and rule/approval state;
3. delivery record existence and status;
4. `dispatch_queued` evidence;
5. Redis/Asynq queue, scheduled, active, retry, and dead state;
6. Delivery claim and lease;
7. Telegram send attempt and provider response.

Do not restart collection or re-fetch source messages until the collection-to-delivery handoff has been shown to be missing or invalid. A Telegram send failure is a Delivery/Publish incident even when the original material came from the Collector.

## Architecture review gates

The repository guard scans both `addons/telegram_collector` and
`addons/youban_publish`. Collector code is not allowed to import Publish
implementation packages or query Telegram Job/Outbox state. A collection
query must carry a scope predicate and a bounded result (`LIMIT`, or an
equivalent repository method); an unbounded raw query over collection events
is a review failure. These checks protect the hot path without requiring a
database scan during startup.

The queue configuration contract is explicit:

```text
YOUBAN_TELEGRAM_COLLECTOR_INGEST_SPOOL_MODE=redis_first
YOUBAN_PUBLISH_COLLECT_PROCESS_OUTBOX_MODE=redis_first
```

`postgres_outbox` is a migration/recovery mode for legacy rows only. It must
not be enabled for the realtime hot path. Redis/Asynq is the execution
handoff; PostgreSQL remains the durable source of collection facts.

The guard does not claim that every existing compatibility query is optimal.
It prevents new global scans and prevents collection success from being
implemented by reading or mutating TG Job state. Existing exceptions, if any,
must be represented by an exact baseline entry with a reviewed function and
expression digest.

## Current runtime guard

The implementation has a compatibility bridge between the collector event store and
publish preparation. `telegram_collector` starts `StartRuntime` for
`collector-worker`, while `StartDeliveryRuntime` is started only for
`push-worker` (or the combined `worker` role). The latter consumes the collector
handoff task and invokes the registered publish adapter.

That adapter is still collection-side work: it persists the normalized material,
applies collection and approval rules, creates the durable publish dispatch, and
hands the work to the publish queue. It must stop at that handoff. It must not
call Telegram send APIs, wait for send confirmation, or set a final publication
state. Telegram API calls and send retries remain in the publish/TG worker.

The collector package has an architecture guard test that rejects direct imports
of `youban_publish` and direct Telegram send method calls. Role gating is also
covered: a collector-only deployment must not start the delivery runtime. Any
change that moves a send API or publish worker startup into the collector role is
an architecture violation and should be rejected during review.
