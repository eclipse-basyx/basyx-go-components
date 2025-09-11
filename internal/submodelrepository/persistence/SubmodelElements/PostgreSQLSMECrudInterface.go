package submodelelements

type PostgreSQLSMECrudInterface interface {
	Create(string, interface{}) error
	Read(string) error
	Update(string, interface{}) error
	Delete(string) error
}
