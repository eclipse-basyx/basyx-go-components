package persistence

import model "github.com/eclipse-basyx/basyx-go-sdk/pkg/submodelrepositoryapi/go"

type SubmodelDatabase interface {
	GetAllSubmodels() ([]model.Submodel, error)
	GetSubmodel(id string) (model.Submodel, error)
	CreateSubmodel(submodel model.Submodel) (string, error)
	DeleteSubmodel(id string) error
}
