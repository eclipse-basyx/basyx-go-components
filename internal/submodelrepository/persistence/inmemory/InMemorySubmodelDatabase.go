package persistence_inmemory

import (
	"errors"

	model "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
)

type InMemorySubmodelDatabase struct {
	submodels map[string]model.Submodel
}

var (
	ErrSubmodelAlreadyExists = errors.New("submodel already exists")
	ErrSubmodelNotFound      = errors.New("submodel not found")
)

// NewInMemorySubmodelDatabase creates a new in-memory submodel database
func NewInMemorySubmodelDatabase() (*InMemorySubmodelDatabase, error) {
	return &InMemorySubmodelDatabase{
		submodels: make(map[string]model.Submodel),
	}, nil
}

// GetAllSubmodels returns all submodels in the database
func (db *InMemorySubmodelDatabase) GetAllSubmodels() ([]model.Submodel, error) {
	var submodels []model.Submodel
	for _, submodel := range db.submodels {
		submodels = append(submodels, submodel)
	}
	return submodels, nil
}

// GetSubmodel returns a submodel by its ID
func (db *InMemorySubmodelDatabase) GetSubmodel(id string) (model.Submodel, error) {
	submodel, exists := db.submodels[id]
	if !exists {
		return model.Submodel{}, ErrSubmodelNotFound
	}
	return submodel, nil
}

// CreateSubmodel creates a new submodel in the database
func (db *InMemorySubmodelDatabase) CreateSubmodel(submodel model.Submodel) (string, error) {
	if _, exists := db.submodels[submodel.Id]; exists {
		return "", ErrSubmodelAlreadyExists
	}
	db.submodels[submodel.Id] = submodel
	return submodel.Id, nil
}

// DeleteSubmodel deletes a submodel by its ID
func (db *InMemorySubmodelDatabase) DeleteSubmodel(id string) error {
	if _, exists := db.submodels[id]; !exists {
		return ErrSubmodelNotFound
	}
	delete(db.submodels, id)
	return nil
}
