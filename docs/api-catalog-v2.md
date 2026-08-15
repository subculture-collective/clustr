# Public Catalog v2 API

All Catalog responses are pinned to an immutable graph revision and its exact
Catalog. If `revision` is omitted, the server resolves it once from the current
published pointer. Opaque cursors are HMAC-signed and bound to revision,
Catalog, query, filters, and sort; changing any bound value returns `409`.

## Endpoints

- `GET /api/graph/catalog` accepts `revision`, `q`, `type`, `entity_id`,
  `community_id`, `macro_group_id`, `parent_id`, `author_id`, `relation`,
  `sensitive`, `sort`, `limit`, and `cursor`. Limits default to 50 and cap at
  200. Sorts are `relevance`, `activity`, `size`, `distinctiveness`, and
  `newest`.
- `GET /api/graph/catalog/facets` returns exact visible totals by entity type.
- `GET /api/graph/catalog/tree/{id}` returns exactly one containment level plus
  cross-linked authors. Containment is cluster to subreddit to post to comment
  to reply; users are never emitted as false children.
- `GET /api/graph/macro-groups` returns stable group IDs, names, palette roles,
  representatives, metrics, and relationship-derived presentation fields.
- `GET /api/nodes/{id}` includes public document fields, community/macro-group,
  distinctiveness, affinity confidence, representative status, sensitive
  state, evidence composition, and Locate/Open/Copy/source actions.

Active global suppression is applied at read time to all endpoints. Direct
access to a suppressed entity returns `410 Gone` without a reason. Lists,
relationships, facets, telemetry, HTML, JSON-LD, and sitemaps omit it and any
associated public link.

Stored content is plain text. Sensitive full text is flagged and receives no
search snippet; clients must keep it collapsed behind a warning. Removed and
deleted source text is never copied into an immutable Catalog.

The established graph fields remain available during the rollback window.
`spatial-scene-v2` adds normalized `affinity`, observed evidence, signal
composition, automatic/evidence names, macro-group colors, and bridge state.
Clients accepting v1 use evidence labels and deterministic hash colors when v2
presentation fields are absent.
