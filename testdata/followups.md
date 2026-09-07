# Follow-ups

## Observability

- **Database tracing waits on the Postgres port.** decision 0030 covers HTTP
  requests and jobs, not the store.
- **The Datadog API key is resolved.** FIXED in commit 2588462.
- **The agent has no span of its own.** The worker ran untraced for a week.
  **FIXED** `2588462` (2026-08-05), one span per run.

## Housekeeping

- [ ] a plain checkbox still counts
