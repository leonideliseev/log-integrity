package discovery

import (
	"github.com/lenchik/logmonitor/internal/ssh"
	"github.com/lenchik/logmonitor/models"
)

// DiscoveredLog содержит информацию об обнаруженном журнале
type DiscoveredLog struct {
	Path    string         // абсолютный путь к файлу журнала на удалённом сервере
	LogType models.LogType // определённый тип журнала
}

// Discoverer — интерфейс для обнаружения журналов на удалённом сервере.
// Каждая ОС имеет свою реализацию.
type Discoverer interface {
	// Discover подключается к серверу через SSH и возвращает список обнаруженных журналов
	Discover(client ssh.Client) ([]DiscoveredLog, error)

	// SupportedOS возвращает тип ОС, для которой предназначен данный Discoverer
	SupportedOS() models.OSType
}

// Registry хранит зарегистрированные Discoverer-ы по типу ОС
type Registry struct {
	discoverers map[models.OSType]Discoverer
}

// NewRegistry создаёт новый реестр Discoverer-ов
// NewRegistry creates an empty discoverer registry.
func NewRegistry() *Registry {
	return &Registry{
		discoverers: make(map[models.OSType]Discoverer),
	}
}

// Register регистрирует Discoverer для указанной ОС
// Register stores a discoverer for its supported operating system.
func (r *Registry) Register(d Discoverer) {
	r.discoverers[d.SupportedOS()] = d
}

// Get возвращает Discoverer для указанной ОС.
// Если Discoverer не найден — возвращает nil, false.
// Get returns a discoverer for the requested operating system.
func (r *Registry) Get(osType models.OSType) (Discoverer, bool) {
	d, ok := r.discoverers[osType]
	return d, ok
}
