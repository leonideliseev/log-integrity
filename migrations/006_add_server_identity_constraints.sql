-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS ux_servers_name_lower
	ON servers(LOWER(name));

CREATE UNIQUE INDEX IF NOT EXISTS ux_servers_host_lower
	ON servers(LOWER(host));

-- +goose Down
DROP INDEX IF EXISTS ux_servers_host_lower;
DROP INDEX IF EXISTS ux_servers_name_lower;
