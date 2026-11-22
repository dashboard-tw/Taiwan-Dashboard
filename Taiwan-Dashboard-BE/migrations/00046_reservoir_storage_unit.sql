-- +goose Up
UPDATE data.reservoir_obs_history h
   SET effective_storage = h.effective_storage / 10000,
       storage_pct = round(h.effective_storage / 10000 / r.effective_capacity * 100, 2)
  FROM ref.reservoir r
 WHERE r.reservoir_id = h.reservoir_id AND r.effective_capacity > 0
   AND h.effective_storage > r.effective_capacity * 10
   AND h.effective_storage / 10000 <= r.effective_capacity * 1.5;

UPDATE data.reservoir_obs_current h
   SET effective_storage = h.effective_storage / 10000,
       storage_pct = round(h.effective_storage / 10000 / r.effective_capacity * 100, 2)
  FROM ref.reservoir r
 WHERE r.reservoir_id = h.reservoir_id AND r.effective_capacity > 0
   AND h.effective_storage > r.effective_capacity * 10
   AND h.effective_storage / 10000 <= r.effective_capacity * 1.5;

-- +goose Down
SELECT 1;
