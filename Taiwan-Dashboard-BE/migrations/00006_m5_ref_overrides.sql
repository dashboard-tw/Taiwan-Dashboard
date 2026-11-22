-- +goose Up
ALTER TABLE ref.reservoir
    ADD COLUMN capacity_locked boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN ref.reservoir.capacity_locked IS '管理員手動設定有效容量後為 true，資料管線不再覆寫';

-- +goose Down
ALTER TABLE ref.reservoir DROP COLUMN capacity_locked;
