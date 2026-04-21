package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/lenchik/logmonitor/models"
)

// CreateServer stores a new server model in PostgreSQL.
func (s *Storage) CreateServer(ctx context.Context, serverModel *models.Server) error {
	if serverModel.ID == "" {
		serverModel.ID = newID("srv")
	}

	now := time.Now().UTC()
	if serverModel.CreatedAt.IsZero() {
		serverModel.CreatedAt = now
	}
	if serverModel.UpdatedAt.IsZero() {
		serverModel.UpdatedAt = serverModel.CreatedAt
	}
	if serverModel.Status == "" {
		serverModel.Status = models.ServerStatusInactive
	}
	if serverModel.ManagedBy == "" {
		serverModel.ManagedBy = models.ServerManagedByAPI
	}
	authValue, err := s.encryptAuthValue(serverModel.AuthValue)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(
		ctx,
		`INSERT INTO servers (id, name, host, port, username, auth_type, auth_value, os_type, status, managed_by, success_count, failure_count, last_error, last_seen_at, backoff_until, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
		serverModel.ID,
		serverModel.Name,
		serverModel.Host,
		serverModel.Port,
		serverModel.Username,
		string(serverModel.AuthType),
		authValue,
		string(serverModel.OSType),
		string(serverModel.Status),
		string(serverModel.ManagedBy),
		serverModel.SuccessCount,
		serverModel.FailureCount,
		serverModel.LastError,
		serverModel.LastSeenAt,
		serverModel.BackoffUntil,
		serverModel.CreatedAt,
		serverModel.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return conflictError("server name, host or id already exists")
		}
		return fmt.Errorf("postgres: create server %q: %w", serverModel.ID, err)
	}

	return nil
}

// GetServerByID returns a server by its identifier.
func (s *Storage) GetServerByID(ctx context.Context, id string) (*models.Server, error) {
	row := s.pool.QueryRow(
		ctx,
		`SELECT id, name, host, port, username, auth_type, auth_value, os_type, status, managed_by, success_count, failure_count, last_error, last_seen_at, backoff_until, created_at, updated_at
		 FROM servers
		 WHERE id = $1`,
		id,
	)

	var serverModel models.Server
	var authType string
	var osType string
	var status string
	var managedBy string
	var lastSeenAt pgtype.Timestamptz
	var backoffUntil pgtype.Timestamptz
	if err := row.Scan(
		&serverModel.ID,
		&serverModel.Name,
		&serverModel.Host,
		&serverModel.Port,
		&serverModel.Username,
		&authType,
		&serverModel.AuthValue,
		&osType,
		&status,
		&managedBy,
		&serverModel.SuccessCount,
		&serverModel.FailureCount,
		&serverModel.LastError,
		&lastSeenAt,
		&backoffUntil,
		&serverModel.CreatedAt,
		&serverModel.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, serverNotFoundError(id)
		}
		return nil, fmt.Errorf("postgres: get server %q: %w", id, err)
	}

	serverModel.AuthType = models.AuthType(authType)
	serverModel.OSType = models.OSType(osType)
	serverModel.Status = models.ServerStatus(status)
	serverModel.ManagedBy = normalizeServerManagedBy(managedBy)
	serverModel.LastSeenAt = nullableTime(lastSeenAt)
	serverModel.BackoffUntil = nullableTime(backoffUntil)
	if err := s.decryptServerAuthValue(&serverModel); err != nil {
		return nil, err
	}
	return cloneServer(&serverModel), nil
}

// ListServers returns all stored servers ordered by name.
func (s *Storage) ListServers(ctx context.Context) ([]*models.Server, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, name, host, port, username, auth_type, auth_value, os_type, status, managed_by, success_count, failure_count, last_error, last_seen_at, backoff_until, created_at, updated_at
		 FROM servers
		 ORDER BY name ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: list servers: %w", err)
	}
	defer rows.Close()

	items := make([]*models.Server, 0)
	for rows.Next() {
		var serverModel models.Server
		var authType string
		var osType string
		var status string
		var managedBy string
		var lastSeenAt pgtype.Timestamptz
		var backoffUntil pgtype.Timestamptz
		if err := rows.Scan(
			&serverModel.ID,
			&serverModel.Name,
			&serverModel.Host,
			&serverModel.Port,
			&serverModel.Username,
			&authType,
			&serverModel.AuthValue,
			&osType,
			&status,
			&managedBy,
			&serverModel.SuccessCount,
			&serverModel.FailureCount,
			&serverModel.LastError,
			&lastSeenAt,
			&backoffUntil,
			&serverModel.CreatedAt,
			&serverModel.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan server: %w", err)
		}

		serverModel.AuthType = models.AuthType(authType)
		serverModel.OSType = models.OSType(osType)
		serverModel.Status = models.ServerStatus(status)
		serverModel.ManagedBy = normalizeServerManagedBy(managedBy)
		serverModel.LastSeenAt = nullableTime(lastSeenAt)
		serverModel.BackoffUntil = nullableTime(backoffUntil)
		if err := s.decryptServerAuthValue(&serverModel); err != nil {
			return nil, err
		}
		items = append(items, cloneServer(&serverModel))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: iterate servers: %w", err)
	}

	return items, nil
}

// UpdateServer overwrites an existing server model.
func (s *Storage) UpdateServer(ctx context.Context, serverModel *models.Server) error {
	if serverModel.ManagedBy == "" {
		serverModel.ManagedBy = models.ServerManagedByAPI
	}
	authValue, err := s.encryptAuthValue(serverModel.AuthValue)
	if err != nil {
		return err
	}

	result, err := s.pool.Exec(
		ctx,
		`UPDATE servers
		 SET name = $2,
		     host = $3,
		     port = $4,
		     username = $5,
		     auth_type = $6,
		     auth_value = $7,
		     os_type = $8,
		     status = $9,
		     managed_by = $10,
		     success_count = $11,
		     failure_count = $12,
		     last_error = $13,
		     last_seen_at = $14,
		     backoff_until = $15,
		     created_at = $16,
		     updated_at = $17
		 WHERE id = $1`,
		serverModel.ID,
		serverModel.Name,
		serverModel.Host,
		serverModel.Port,
		serverModel.Username,
		string(serverModel.AuthType),
		authValue,
		string(serverModel.OSType),
		string(serverModel.Status),
		string(serverModel.ManagedBy),
		serverModel.SuccessCount,
		serverModel.FailureCount,
		serverModel.LastError,
		serverModel.LastSeenAt,
		serverModel.BackoffUntil,
		serverModel.CreatedAt,
		time.Now().UTC(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return conflictError("server name, host or id already exists")
		}
		return fmt.Errorf("postgres: update server %q: %w", serverModel.ID, err)
	}

	if result.RowsAffected() == 0 {
		return serverNotFoundError(serverModel.ID)
	}

	return nil
}

func (s *Storage) encryptAuthValue(value string) (string, error) {
	encrypted, err := s.authCipher.Encrypt(value)
	if err != nil {
		return "", fmt.Errorf("postgres: encrypt auth value: %w", err)
	}
	return encrypted, nil
}

func (s *Storage) decryptServerAuthValue(serverModel *models.Server) error {
	decrypted, err := s.authCipher.Decrypt(serverModel.AuthValue)
	if err != nil {
		return fmt.Errorf("postgres: decrypt server auth value %q: %w", serverModel.ID, err)
	}
	serverModel.AuthValue = decrypted
	return nil
}

// normalizeServerManagedBy keeps legacy rows compatible after adding server ownership.
func normalizeServerManagedBy(value string) models.ServerManagedBy {
	if value == "" {
		return models.ServerManagedByConfig
	}
	return models.ServerManagedBy(value)
}

// DeleteServer removes a server and every dependent entity linked to it.
func (s *Storage) DeleteServer(ctx context.Context, id string) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete server %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return serverNotFoundError(id)
	}

	return nil
}

// UpdateServerStatus updates only the server status field.
func (s *Storage) UpdateServerStatus(ctx context.Context, id string, status models.ServerStatus) error {
	result, err := s.pool.Exec(
		ctx,
		`UPDATE servers
		 SET status = $2,
		     updated_at = $3
		 WHERE id = $1`,
		id,
		string(status),
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres: update server status %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return serverNotFoundError(id)
	}

	return nil
}

// RecordServerSuccess records a successful remote server operation.
func (s *Storage) RecordServerSuccess(ctx context.Context, id string, seenAt time.Time) error {
	result, err := s.pool.Exec(
		ctx,
		`UPDATE servers
		 SET status = $2,
		     success_count = success_count + 1,
		     failure_count = 0,
		     last_error = '',
		     last_seen_at = $3,
		     backoff_until = NULL,
		     updated_at = $4
		 WHERE id = $1`,
		id,
		string(models.ServerStatusActive),
		seenAt,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres: record server success %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return serverNotFoundError(id)
	}

	return nil
}

// RecordServerFailure records a failed remote server operation.
func (s *Storage) RecordServerFailure(ctx context.Context, id string, lastError string, backoffUntil *time.Time) error {
	result, err := s.pool.Exec(
		ctx,
		`UPDATE servers
		 SET status = $2,
		     failure_count = failure_count + 1,
		     last_error = $3,
		     backoff_until = $4,
		     updated_at = $5
		 WHERE id = $1`,
		id,
		string(models.ServerStatusError),
		lastError,
		backoffUntil,
		time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("postgres: record server failure %q: %w", id, err)
	}

	if result.RowsAffected() == 0 {
		return serverNotFoundError(id)
	}

	return nil
}
