-- A crawl request expresses the durable desire to keep a subreddit fresh.  It
-- deliberately outlives individual attempts, unlike the legacy crawl_jobs row.
CREATE TABLE IF NOT EXISTS crawl_requests (
    id BIGSERIAL PRIMARY KEY,
    subreddit_id INTEGER NOT NULL UNIQUE REFERENCES subreddits(id) ON DELETE CASCADE,
    enabled BOOLEAN NOT NULL DEFAULT true,
    activation_cohort TEXT NOT NULL DEFAULT 'active' CHECK (activation_cohort IN ('active', 'backlog')),
    priority INTEGER NOT NULL DEFAULT 0,
    desired_freshness INTERVAL NOT NULL DEFAULT INTERVAL '7 days',
    next_due_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    discovery_provenance TEXT NOT NULL DEFAULT 'legacy',
    last_completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (desired_freshness > INTERVAL '0 seconds')
);

CREATE INDEX IF NOT EXISTS idx_crawl_requests_due
    ON crawl_requests (activation_cohort, next_due_at, priority DESC) WHERE enabled;

CREATE TABLE IF NOT EXISTS crawl_attempts (
    id BIGSERIAL PRIMARY KEY,
    request_id BIGINT NOT NULL REFERENCES crawl_requests(id) ON DELETE CASCADE,
    state TEXT NOT NULL CHECK (state IN ('leased', 'complete', 'partial', 'retryable_failure', 'permanent_failure', 'cancelled', 'legacy')),
    worker_id TEXT,
    lease_expires_at TIMESTAMPTZ,
    retry_number INTEGER NOT NULL DEFAULT 0 CHECK (retry_number >= 0),
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ,
    posts_count INTEGER NOT NULL DEFAULT 0,
    comments_count INTEGER NOT NULL DEFAULT 0,
    item_failures INTEGER NOT NULL DEFAULT 0,
    error_class TEXT,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((state = 'leased') = (finished_at IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_crawl_attempts_lease
    ON crawl_attempts (lease_expires_at) WHERE state = 'leased';
CREATE INDEX IF NOT EXISTS idx_crawl_attempts_request
    ON crawl_attempts (request_id, started_at DESC);

CREATE TABLE IF NOT EXISTS discovery_candidates (
    id BIGSERIAL PRIMARY KEY,
    subreddit_id INTEGER NOT NULL REFERENCES subreddits(id) ON DELETE CASCADE,
    source TEXT NOT NULL CHECK (source IN ('mention', 'author_history')),
    source_entity TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    disposition TEXT NOT NULL DEFAULT 'pending' CHECK (disposition IN ('pending', 'promoted', 'rejected', 'deferred')),
    UNIQUE (subreddit_id, source, source_entity)
);

CREATE INDEX IF NOT EXISTS idx_discovery_candidates_pending
    ON discovery_candidates (disposition, score DESC, last_seen_at DESC) WHERE disposition = 'pending';

-- Every imported request is assigned to the backlog cohort unless it is a
-- clearly explicit queued/crawling API/manual request. The worker centrally
-- limits backlog activations. A deterministic seven-day due spread avoids a
-- large legacy queue becoming immediately eligible before that throttle acts.
INSERT INTO crawl_requests (subreddit_id, enabled, activation_cohort, priority, next_due_at, discovery_provenance, last_completed_at)
SELECT s.id,
       true,
	   CASE WHEN j.status IN ('queued', 'crawling') AND lower(COALESCE(j.enqueued_by, '')) IN ('api', 'manual') THEN 'active' ELSE 'backlog' END,
       COALESCE(j.priority, 0),
       CASE
         WHEN j.status IN ('queued', 'crawling') AND lower(COALESCE(j.enqueued_by, '')) IN ('api', 'manual') THEN now()
         ELSE now() + make_interval(secs => mod(abs(hashtext('crawl-request:' || s.id::text)::bigint), 604800)::integer)
       END,
       COALESCE(j.enqueued_by, 'legacy'),
       CASE WHEN j.status = 'success' THEN j.updated_at ELSE NULL END
FROM subreddits s
LEFT JOIN crawl_jobs j ON j.subreddit_id = s.id
ON CONFLICT (subreddit_id) DO NOTHING;

INSERT INTO crawl_attempts (request_id, state, retry_number, started_at, finished_at, error_class, error_message)
SELECT r.id,
       'legacy',
       COALESCE(j.retries, 0),
       COALESCE(j.last_attempt, j.created_at, now()),
       COALESCE(j.updated_at, now()),
       CASE WHEN j.status = 'failed' THEN 'legacy_failure' ELSE NULL END,
       'Imported from crawl_jobs'
FROM crawl_jobs j
JOIN crawl_requests r ON r.subreddit_id = j.subreddit_id
WHERE NOT EXISTS (
  SELECT 1 FROM crawl_attempts a
  WHERE a.request_id = r.id AND a.state = 'legacy'
);
