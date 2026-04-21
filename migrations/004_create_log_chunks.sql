-- +goose Up
CREATE TABLE IF NOT EXISTS log_chunks (
	id TEXT PRIMARY KEY,
	log_file_id TEXT NOT NULL REFERENCES log_files(id) ON DELETE CASCADE,
	chunk_number BIGINT NOT NULL,
	from_line_number BIGINT NOT NULL,
	to_line_number BIGINT NOT NULL,
	from_byte_offset BIGINT NOT NULL DEFAULT 0,
	to_byte_offset BIGINT NOT NULL DEFAULT 0,
	entries_count INTEGER NOT NULL,
	hash TEXT NOT NULL,
	hash_algorithm TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_log_chunks_log_file_number
	ON log_chunks(log_file_id, chunk_number);

CREATE INDEX IF NOT EXISTS ix_log_chunks_log_file_range
	ON log_chunks(log_file_id, from_line_number, to_line_number);

-- +goose Down
DROP TABLE IF EXISTS log_chunks;
