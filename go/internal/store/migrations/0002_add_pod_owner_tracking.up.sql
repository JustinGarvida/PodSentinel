-- Tracks each pod's UID and controlling owner so a replaced pod can be correlated back to its workload.
ALTER TABLE pod_metrics
    ADD COLUMN IF NOT EXISTS pod_uid    TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS owner_kind TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS owner_name TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS pod_metrics_owner_time_idx
    ON pod_metrics (namespace, owner_kind, owner_name, time DESC);
