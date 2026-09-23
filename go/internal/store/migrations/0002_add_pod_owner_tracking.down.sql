DROP INDEX IF EXISTS pod_metrics_owner_time_idx;

ALTER TABLE pod_metrics
    DROP COLUMN IF EXISTS pod_uid,
    DROP COLUMN IF EXISTS owner_kind,
    DROP COLUMN IF EXISTS owner_name;
