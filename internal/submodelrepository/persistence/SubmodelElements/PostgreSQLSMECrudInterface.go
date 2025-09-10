package submodelelements

import gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"

type PostgreSQLSMECrudInterface interface {
	Create(string, gen.SubmodelElement) error
	Read(string) error
	Update(string, gen.SubmodelElement) error
	Delete(string) error
}
