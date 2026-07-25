package organization

import "github.com/PangIkp/devlens/backend/internal/postgres"

// Repository owns PostgreSQL access for organization use cases.
type Repository struct {
	db *postgres.DB
}

func NewRepository(db *postgres.DB) *Repository {
	return &Repository{db: db}
}
