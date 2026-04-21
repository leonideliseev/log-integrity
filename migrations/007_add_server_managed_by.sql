-- +goose Up
ALTER TABLE servers
	ADD COLUMN IF NOT EXISTS managed_by TEXT NOT NULL DEFAULT 'config';

CREATE INDEX IF NOT EXISTS idx_servers_managed_by
	ON servers (managed_by);

-- +goose Down
DROP INDEX IF EXISTS idx_servers_managed_by;

ALTER TABLE servers
	DROP COLUMN IF EXISTS managed_by;
