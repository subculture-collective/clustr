# ADR 0004: Separate recurring crawl requests from attempts

- Status: accepted
- Date: 2026-08-12

## Context

One terminal queue row per subreddit prevents later successful recrawls and
splits retry state across incompatible counters. Process-local discovery state
cannot survive restarts or explain why a source was promoted.

## Decision

A `crawl_request` is the idempotent recurring freshness intent for one
subreddit. Each execution is an append-only `crawl_attempt`, claimed with
`FOR UPDATE SKIP LOCKED`, a renewable two-minute lease, and a 30-second
heartbeat. There are three attempts total; retry delay starts at one minute,
uses exponential jitter, and never exceeds 24 hours. Completion schedules the
next due time.

All scheduled, stale, manual, mention, and author-history inputs use the same
lifecycle Module. Outcomes are complete, partial, retryable failure, permanent
failure, or cancelled. Discovery candidates persist provenance, score,
first/last seen, and disposition; configured daily and per-crawl budgets are
enforced centrally.

## Consequences

Legacy `crawl_jobs` routes remain a compatibility Adapter while
`CRAWLER_LIFECYCLE_ENABLED` controls worker authority. Shadow comparison and
lease-recovery tests are required before the legacy tables become read-only.
