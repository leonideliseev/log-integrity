-- +goose Up
CREATE TABLE IF NOT EXISTS log_files (
	id TEXT PRIMARY KEY,
	server_id TEXT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	log_type TEXT NOT NULL,
	file_identity JSONB NOT NULL DEFAULT '{}'::jsonb,
	meta JSONB NOT NULL DEFAULT '{}'::jsonb,
	last_scanned_at TIMESTAMPTZ NULL,
	last_line_number BIGINT NOT NULL DEFAULT 0,
	last_byte_offset BIGINT NOT NULL DEFAULT 0,
	is_active BOOLEAN NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_log_files_server_path
	ON log_files(server_id, path);

CREATE INDEX IF NOT EXISTS ix_log_files_active
	ON log_files(is_active, server_id, path);

CREATE INDEX IF NOT EXISTS ix_log_files_server
	ON log_files(server_id, path);

-- +goose Down
DROP TABLE IF EXISTS log_files;
