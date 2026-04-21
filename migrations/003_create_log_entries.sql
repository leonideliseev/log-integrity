-- +goose Up
CREATE TABLE IF NOT EXISTS log_entries (
	id TEXT PRIMARY KEY,
	log_file_id TEXT NOT NULL REFERENCES log_files(id) ON DELETE CASCADE,
	line_number BIGINT NOT NULL,
	content TEXT NOT NULL,
	hash TEXT NOT NULL,
	collected_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_log_entries_log_file_line
	ON log_entries(log_file_id, line_number);

CREATE INDEX IF NOT EXISTS ix_log_entries_log_file_line
	ON log_entries(log_file_id, line_number);

-- +goose Down
DROP TABLE IF EXISTS log_entries;
