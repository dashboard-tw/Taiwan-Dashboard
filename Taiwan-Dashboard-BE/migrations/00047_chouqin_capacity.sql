-- +goose Up
UPDATE ref.reservoir
   SET effective_capacity = 7.31, capacity_locked = true,
       capacity_updated_on = (now() AT TIME ZONE 'Asia/Taipei')::date, updated_at = now()
 WHERE reservoir_id = '40701' AND effective_capacity IS NULL;

UPDATE data.reservoir_obs_history
   SET storage_pct = round(effective_storage / 7.31 * 100, 2)
 WHERE reservoir_id = '40701' AND effective_storage IS NOT NULL AND storage_pct IS NULL;

UPDATE data.reservoir_obs_current
   SET storage_pct = round(effective_storage / 7.31 * 100, 2)
 WHERE reservoir_id = '40701' AND effective_storage IS NOT NULL AND storage_pct IS NULL;

-- +goose Down
UPDATE data.reservoir_obs_current SET storage_pct = NULL WHERE reservoir_id = '40701';
UPDATE data.reservoir_obs_history SET storage_pct = NULL WHERE reservoir_id = '40701';
UPDATE ref.reservoir SET effective_capacity = NULL, capacity_locked = false
 WHERE reservoir_id = '40701' AND effective_capacity = 7.31;
