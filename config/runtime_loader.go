package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-([^}]*))?}`)

// LoadRuntime loads the runtime YAML configuration and applies defaults.
func LoadRuntime(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %q: %w", path, err)
	}
	data = []byte(expandEnvDefaults(string(data)))

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	applyRuntimeDefaults(cfg)
	if err := validateRuntime(cfg); err != nil {
		return nil, fmt.Errorf("config: validate: %w", err)
	}

	return cfg, nil
}

// expandEnvDefaults replaces ${VAR} and ${VAR:-default} placeholders in YAML content.
func expandEnvDefaults(value string) string {
	return envPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := envPattern.FindStringSubmatch(match)
		if len(parts) == 0 {
			return match
		}

		envValue, ok := os.LookupEnv(parts[1])
		if ok {
			return envValue
		}
		if len(parts) >= 4 && strings.HasPrefix(parts[2], ":-") {
			return parts[3]
		}
		return ""
	})
}

// applyRuntimeDefaults fills optional config fields with fallback values.
func applyRuntimeDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "0.0.0.0"
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Database.Port == 0 {
		cfg.Database.Port = 5432
	}
	if cfg.Database.SSLMode == "" {
		cfg.Database.SSLMode = "disable"
	}
	if cfg.Database.MigrationsDir == "" {
		cfg.Database.MigrationsDir = "migrations"
	}
	if cfg.Database.MaxConns == 0 {
		cfg.Database.MaxConns = 10
	}
	if cfg.Database.MinConns == 0 {
		cfg.Database.MinConns = 1
	}
	if cfg.SSH.ConnectTimeoutSeconds == 0 {
		cfg.SSH.ConnectTimeoutSeconds = 10
	}
	if cfg.SSH.CommandTimeoutSeconds == 0 {
		cfg.SSH.CommandTimeoutSeconds = 30
	}
	if cfg.SSH.InsecureIgnoreHostKey == nil {
		defaultInsecureIgnoreHostKey := false
		cfg.SSH.InsecureIgnoreHostKey = &defaultInsecureIgnoreHostKey
	}
	if cfg.SSH.KnownHostsPath == "" && !*cfg.SSH.InsecureIgnoreHostKey {
		cfg.SSH.KnownHostsPath = defaultKnownHostsPath()
	}
	if cfg.Scheduler.DiscoveryCron == "" {
		cfg.Scheduler.DiscoveryCron = "0 */6 * * *"
	}
	if cfg.Scheduler.CollectionCron == "" {
		cfg.Scheduler.CollectionCron = "*/5 * * * *"
	}
	if cfg.Scheduler.IntegrityCron == "" {
		cfg.Scheduler.IntegrityCron = "0 * * * *"
	}
	if cfg.Collector.BatchSize == 0 {
		cfg.Collector.BatchSize = 5000
	}
	if cfg.Collector.ChunkSize == 0 {
		cfg.Collector.ChunkSize = 1000
	}
	if cfg.Collector.ChunkHashAlgo == "" {
		cfg.Collector.ChunkHashAlgo = "sha256"
	}
	if cfg.Collector.StoreRawContent == nil {
		defaultStoreRawContent := true
		cfg.Collector.StoreRawContent = &defaultStoreRawContent
	}
	if cfg.Workers.DiscoveryServers == 0 {
		cfg.Workers.DiscoveryServers = 4
	}
	if cfg.Workers.CollectionServers == 0 {
		cfg.Workers.CollectionServers = 4
	}
	if cfg.Workers.CollectionLogFilesPerHost == 0 {
		cfg.Workers.CollectionLogFilesPerHost = 2
	}
	if cfg.Workers.IntegrityServers == 0 {
		cfg.Workers.IntegrityServers = 2
	}
	if cfg.Workers.IntegrityLogFilesPerHost == 0 {
		cfg.Workers.IntegrityLogFilesPerHost = 1
	}
	if cfg.Workers.PerServerIsolation == nil {
		defaultPerServerIsolation := true
		cfg.Workers.PerServerIsolation = &defaultPerServerIsolation
	}
}

// validateRuntime validates only the fields required for the current runtime mode.
func validateRuntime(cfg *Config) error {
	if cfg.Database.MaxConns < 0 {
		return fmt.Errorf("database.max_conns must be greater than or equal to zero")
	}
	if cfg.Database.MinConns < 0 {
		return fmt.Errorf("database.min_conns must be greater than or equal to zero")
	}
	if cfg.Database.MaxConns > 0 && cfg.Database.MinConns > cfg.Database.MaxConns {
		return fmt.Errorf("database.min_conns must be less than or equal to database.max_conns")
	}
	if cfg.Security.IntegrityHMACKey == "" {
		return fmt.Errorf("security.integrity_hmac_key is required")
	}
	if len(cfg.Security.IntegrityHMACKey) < 16 {
		return fmt.Errorf("security.integrity_hmac_key must contain at least 16 characters")
	}
	if databaseConfigured(cfg) && cfg.Security.AuthValueEncryptionKey == "" {
		return fmt.Errorf("security.auth_value_encryption_key is required when PostgreSQL storage is enabled")
	}
	if cfg.Security.AuthValueEncryptionKey != "" && len(cfg.Security.AuthValueEncryptionKey) < 16 {
		return fmt.Errorf("security.auth_value_encryption_key must contain at least 16 characters")
	}
	if cfg.Collector.BatchSize < 0 {
		return fmt.Errorf("collector.batch_size must be greater than or equal to zero")
	}
	if cfg.Collector.ChunkSize < 0 {
		return fmt.Errorf("collector.chunk_size must be greater than or equal to zero")
	}
	if cfg.Collector.ChunkHashAlgo != "" && cfg.Collector.ChunkHashAlgo != "sha256" {
		return fmt.Errorf("collector.chunk_hash_algo %q is not supported", cfg.Collector.ChunkHashAlgo)
	}
	if cfg.SSH.ConnectTimeoutSeconds < 0 {
		return fmt.Errorf("ssh.connect_timeout_seconds must be greater than or equal to zero")
	}
	if cfg.SSH.CommandTimeoutSeconds < 0 {
		return fmt.Errorf("ssh.command_timeout_seconds must be greater than or equal to zero")
	}
	if !*cfg.SSH.InsecureIgnoreHostKey && cfg.SSH.KnownHostsPath == "" {
		return fmt.Errorf("ssh.known_hosts_path is required when ssh.insecure_ignore_host_key is false")
	}
	if cfg.Workers.DiscoveryServers < 0 {
		return fmt.Errorf("workers.discovery_servers must be greater than or equal to zero")
	}
	if cfg.Workers.CollectionServers < 0 {
		return fmt.Errorf("workers.collection_servers must be greater than or equal to zero")
	}
	if cfg.Workers.CollectionLogFilesPerHost < 0 {
		return fmt.Errorf("workers.collection_log_files_per_host must be greater than or equal to zero")
	}
	if cfg.Workers.IntegrityServers < 0 {
		return fmt.Errorf("workers.integrity_servers must be greater than or equal to zero")
	}
	if cfg.Workers.IntegrityLogFilesPerHost < 0 {
		return fmt.Errorf("workers.integrity_log_files_per_host must be greater than or equal to zero")
	}
	for i := range cfg.Servers {
		if cfg.Servers[i].Name == "" {
			return fmt.Errorf("servers[%d].name is required", i)
		}
		if cfg.Servers[i].Host == "" {
			return fmt.Errorf("servers[%d].host is required", i)
		}
		if cfg.Servers[i].Username == "" {
			return fmt.Errorf("servers[%d].username is required", i)
		}
		if cfg.Servers[i].AuthType != "password" && cfg.Servers[i].AuthType != "key" {
			return fmt.Errorf("servers[%d].auth_type must be either password or key", i)
		}
		if cfg.Servers[i].AuthValue == "" {
			return fmt.Errorf("servers[%d].auth_value is required", i)
		}
		if cfg.Servers[i].OSType != "" && cfg.Servers[i].OSType != "linux" && cfg.Servers[i].OSType != "windows" && cfg.Servers[i].OSType != "macos" {
			return fmt.Errorf("servers[%d].os_type must be empty, linux, windows or macos", i)
		}
	}
	return nil
}

func databaseConfigured(cfg *Config) bool {
	return cfg.Database.Host != "" && cfg.Database.User != "" && cfg.Database.DBName != ""
}

// defaultKnownHostsPath returns the standard OpenSSH known_hosts path for the current user.
func defaultKnownHostsPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return ""
	}
	return filepath.Join(homeDir, ".ssh", "known_hosts")
}
