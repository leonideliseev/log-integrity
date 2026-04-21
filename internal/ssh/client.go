// Package ssh contains SSH client abstractions and the default executor.
package ssh

import (
	"context"

	"github.com/lenchik/logmonitor/models"
)

// Client — интерфейс SSH-клиента для подключения к удалённым серверам
// и выполнения команд. Изолирует бизнес-логику от конкретной реализации SSH.
type Client interface {
	// Connect устанавливает SSH-соединение с указанным сервером
	Connect(server *models.Server) error

	// Execute выполняет команду на удалённом сервере и возвращает stdout.
	// Если команда завершилась с ненулевым кодом — возвращает ошибку.
	Execute(cmd string) (string, error)

	// ExecuteContext runs a command and stops waiting when the context is canceled.
	ExecuteContext(ctx context.Context, cmd string) (string, error)

	// Close закрывает SSH-соединение
	Close() error

	// IsConnected возвращает true, если соединение активно
	IsConnected() bool
}

// ClientFactory — фабрика для создания SSH-клиентов.
// Позволяет подменять реализацию в тестах.
type ClientFactory interface {
	NewClient() Client
}
