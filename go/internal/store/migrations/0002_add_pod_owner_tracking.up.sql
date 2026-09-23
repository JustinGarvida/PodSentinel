-- Tracks a pod's stable UID and controlling owner (e.g. Deployment)
-- alongside each sample, so a crashed pod that gets deleted and
-- replaced by a new Pod object (new name, new UID) can still be
-- correlated back to the same workload over time. Distinct from
-- restart_count, which only counts in-place container restarts within
-- one pod object's lifetime.
ALTER TABLE pod_metrics
    ADD COLUMN IF NOT EXISTS pod_uid    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS owner_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS owner_name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS pod_metrics_owner_time_idx
    ON pod_metrics (namespace, owner_kind, owner_name, time DESC);
