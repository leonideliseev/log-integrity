-- +goose Up
CREATE TABLE IF NOT EXISTS servers (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	host TEXT NOT NULL,
	port INTEGER NOT NULL,
	username TEXT NOT NULL,
	auth_type TEXT NOT NULL,
	auth_value TEXT NOT NULL DEFAULT '',
	os_type TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS servers;
