-- +goose Up
ALTER TABLE servers
	ADD COLUMN IF NOT EXISTS success_count BIGINT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS failure_count BIGINT NOT NULL DEFAULT 0,
	ADD COLUMN IF NOT EXISTS last_error TEXT NOT NULL DEFAULT '',
	ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ NULL,
	ADD COLUMN IF NOT EXISTS backoff_until TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_servers_backoff_until
	ON servers (backoff_until);

-- +goose Down
DROP INDEX IF EXISTS idx_servers_backoff_until;

ALTER TABLE servers
	DROP COLUMN IF EXISTS backoff_until,
	DROP COLUMN IF EXISTS last_seen_at,
	DROP COLUMN IF EXISTS last_error,
	DROP COLUMN IF EXISTS failure_count,
	DROP COLUMN IF EXISTS success_count;
