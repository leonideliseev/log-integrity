-- +goose Up
CREATE TABLE IF NOT EXISTS check_results (
	id TEXT PRIMARY KEY,
	log_file_id TEXT NOT NULL REFERENCES log_files(id) ON DELETE CASCADE,
	checked_at TIMESTAMPTZ NOT NULL,
	status TEXT NOT NULL,
	total_lines BIGINT NOT NULL,
	tampered_lines BIGINT NOT NULL,
	error_message TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS ix_check_results_log_file_checked_at
	ON check_results(log_file_id, checked_at, id);

-- +goose Down
DROP TABLE IF EXISTS check_results;
