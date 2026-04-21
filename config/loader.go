package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load читает конфигурационный файл по указанному пути и возвращает Config.
// Если файл не найден или содержит ошибки — возвращает ошибку.
// Load reads a YAML config file and validates the parsed config structure.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: не удалось прочитать файл %q: %w", path, err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: не удалось разобрать YAML: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("config: ошибка валидации: %w", err)
	}

	return cfg, nil
}

// validate проверяет обязательные поля конфигурации
// validate checks required config values for the legacy loader path.
func (c *Config) validate() error {
	if c.Database.Host == "" {
		return fmt.Errorf("database.host не может быть пустым")
	}
	if c.Database.DBName == "" {
		return fmt.Errorf("database.dbname не может быть пустым")
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Database.Port == 0 {
		c.Database.Port = 5432
	}
	if c.Database.SSLMode == "" {
		c.Database.SSLMode = "disable"
	}
	if c.Scheduler.DiscoveryCron == "" {
		c.Scheduler.DiscoveryCron = "0 */6 * * *"
	}
	if c.Scheduler.CollectionCron == "" {
		c.Scheduler.CollectionCron = "*/5 * * * *"
	}
	if c.Scheduler.IntegrityCron == "" {
		c.Scheduler.IntegrityCron = "0 * * * *"
	}
	if c.Health.FailureThreshold == 0 {
		c.Health.FailureThreshold = 1
	}
	if c.Health.BackoffBaseSeconds == 0 {
		c.Health.BackoffBaseSeconds = 60
	}
	if c.Health.BackoffMaxSeconds == 0 {
		c.Health.BackoffMaxSeconds = 900
	}
	if c.Health.LastErrorMaxLength == 0 {
		c.Health.LastErrorMaxLength = 2048
	}
	return nil
}
