package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/lenchik/logmonitor/internal/repository"
	"github.com/lenchik/logmonitor/internal/security"
	"github.com/pressly/goose/v3"
)

// Storage keeps a PostgreSQL-backed implementation of the repository layer.
type Storage struct {
	pool       *pgxpool.Pool
	authCipher *security.StringCipher
}

// Options configures PostgreSQL repository behavior.
type Options struct {
	MaxConns      int32
	MinConns      int32
	MigrationsDir string
	AuthCipher    *security.StringCipher
}

// Open creates a PostgreSQL repository, verifies the connection and prepares the schema.
func Open(dsn string) (repository.Repository, error) {
	return OpenWithOptions(dsn, Options{})
}

// OpenWithOptions creates a PostgreSQL repository with explicit pool and security settings.
func OpenWithOptions(dsn string, options Options) (repository.Repository, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse pool config: %w", err)
	}

	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MinConns = options.MinConns
	cfg.MaxConns = options.MaxConns
	if cfg.MinConns <= 0 {
		cfg.MinConns = 1
	}
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 10
	}
	if cfg.MinConns > cfg.MaxConns {
		cfg.MinConns = cfg.MaxConns
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping database: %w", err)
	}

	if err := runMigrations(dsn, options.MigrationsDir); err != nil {
		pool.Close()
		return nil, err
	}

	store := &Storage{pool: pool, authCipher: options.AuthCipher}
	return store, nil
}

// Close closes the underlying database pool.
func (s *Storage) Close() error {
	s.pool.Close()
	return nil
}

// Ping checks whether the PostgreSQL pool can reach the database.
func (s *Storage) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func runMigrations(dsn, dir string) error {
	if dir == "" {
		dir = "migrations"
	}
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("postgres: parse migration config: %w", err)
	}
	db := stdlib.OpenDB(*connConfig)
	defer func() {
		_ = db.Close()
	}()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("postgres: configure goose dialect: %w", err)
	}
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("postgres: run goose migrations from %q: %w", dir, err)
	}
	return nil
}
