package monitoringdemo

import "github.com/nobiyou/wx_channel_insight/internal/monitoring"

type Repository struct {
	*monitoring.MemoryRepository
}

var _ monitoring.Repository = (*Repository)(nil)

func NewRepository() *Repository {
	return &Repository{MemoryRepository: monitoring.NewMemoryRepository()}
}
