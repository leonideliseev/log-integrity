// Package config describes and loads application configuration.
package config

import "github.com/lenchik/logmonitor/internal/runtimeinfo"

// Config — корневая конфигурация приложения
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	API       APIConfig       `yaml:"api"`
	Security  SecurityConfig  `yaml:"security"`
	Database  DatabaseConfig  `yaml:"database"`
	SSH       SSHConfig       `yaml:"ssh"`
	Scheduler SchedulerConfig `yaml:"scheduler"`
	Collector CollectorConfig `yaml:"collector"`
	Health    HealthConfig    `yaml:"health"`
	Jobs      JobsConfig      `yaml:"jobs"`
	Runtime   RuntimeConfig   `yaml:"runtime"`
	Workers   WorkerConfig    `yaml:"workers"`
	Servers   []ServerEntry   `yaml:"servers"`

	EnvChecks []runtimeinfo.EnvCheck `yaml:"-"`
}

// ServerConfig — настройки HTTP-сервера
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// APIConfig stores HTTP API access settings.
type APIConfig struct {
	AuthToken            string `yaml:"auth_token"`
	AllowUnauthenticated bool   `yaml:"allow_unauthenticated"`
}

// SecurityConfig stores cryptographic keys for persisted secrets and log hashes.
type SecurityConfig struct {
	AuthValueEncryptionKey string `yaml:"auth_value_encryption_key"`
	IntegrityHMACKey       string `yaml:"integrity_hmac_key"`
}

// DatabaseConfig — настройки подключения к PostgreSQL
type DatabaseConfig struct {
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	DBName        string `yaml:"dbname"`
	SSLMode       string `yaml:"sslmode"`
	MaxConns      int32  `yaml:"max_conns"`
	MinConns      int32  `yaml:"min_conns"`
	MigrationsDir string `yaml:"migrations_dir"`
}

// SSHConfig stores remote connection safety and timeout settings.
type SSHConfig struct {
	ConnectTimeoutSeconds int    `yaml:"connect_timeout_seconds"`
	CommandTimeoutSeconds int    `yaml:"command_timeout_seconds"`
	KnownHostsPath        string `yaml:"known_hosts_path"`
	InsecureIgnoreHostKey *bool  `yaml:"insecure_ignore_host_key"`
}

// CollectorConfig stores high-load collection and integrity-preparation settings.
type CollectorConfig struct {
	BatchSize       int    `yaml:"batch_size"`
	ChunkSize       int    `yaml:"chunk_size"`
	StoreRawContent *bool  `yaml:"store_raw_content"`
	ChunkHashAlgo   string `yaml:"chunk_hash_algo"`
}

// RuntimeConfig stores startup behavior toggles used for local and diagnostic runs.
type RuntimeConfig struct {
	DryRun bool `yaml:"dry_run"`
}

// JobsConfig stores async queue sizing and history retention settings.
type JobsConfig struct {
	Workers      int `yaml:"workers"`
	QueueSize    int `yaml:"queue_size"`
	HistoryLimit int `yaml:"history_limit"`
}

// HealthConfig stores server availability lifecycle settings.
type HealthConfig struct {
	FailureThreshold   int `yaml:"failure_threshold"`
	BackoffBaseSeconds int `yaml:"backoff_base_seconds"`
	BackoffMaxSeconds  int `yaml:"backoff_max_seconds"`
	LastErrorMaxLength int `yaml:"last_error_max_length"`
}

// WorkerConfig stores bounded concurrency settings for background jobs.
type WorkerConfig struct {
	DiscoveryServers          int   `yaml:"discovery_servers"`
	CollectionServers         int   `yaml:"collection_servers"`
	CollectionLogFilesPerHost int   `yaml:"collection_log_files_per_host"`
	IntegrityServers          int   `yaml:"integrity_servers"`
	IntegrityLogFilesPerHost  int   `yaml:"integrity_log_files_per_host"`
	PerServerIsolation        *bool `yaml:"per_server_isolation"`
}

// DSN возвращает строку подключения к PostgreSQL
// DSN builds a PostgreSQL connection string from config fields.
func (d DatabaseConfig) DSN() string {
	return "host=" + d.Host +
		" port=" + itoa(d.Port) +
		" user=" + d.User +
		" password=" + d.Password +
		" dbname=" + d.DBName +
		" sslmode=" + d.SSLMode
}

// SchedulerConfig — расписание cron-задач
type SchedulerConfig struct {
	// DiscoveryCron — расписание автоматического обнаружения журналов
	DiscoveryCron string `yaml:"discovery_cron"`
	// CollectionCron — расписание сбора новых записей из журналов
	CollectionCron string `yaml:"collection_cron"`
	// IntegrityCron — расписание проверки целостности
	IntegrityCron string `yaml:"integrity_cron"`
}

// ServerEntry — описание удалённого сервера в конфигурационном файле
type ServerEntry struct {
	Name      string `yaml:"name"`
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	Username  string `yaml:"username"`
	AuthType  string `yaml:"auth_type"`  // "password" или "key"
	AuthValue string `yaml:"auth_value"` // пароль или путь к приватному ключу
	OSType    string `yaml:"os_type"`    // "linux", "windows", "macos" или "" (авто-определение)
}

// itoa — вспомогательная функция для преобразования int в string без импорта strconv
// itoa converts an integer to string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		buf[pos] = byte(n%10) + '0'
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
