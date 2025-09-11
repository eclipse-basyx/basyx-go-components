package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLPropertyHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLPropertyHandler(db *sql.DB) (*PostgreSQLPropertyHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLPropertyHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLPropertyHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	property, ok := submodelElement.(*gen.Property)
	if !ok {
		return 0, errors.New("submodelElement is not of type Property")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Property-specific database insertion
	// Determine which column to use based on valueType
	err = insertProperty(property, err, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLPropertyHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement) (int, error) {
	property, ok := submodelElement.(*gen.Property)
	if !ok {
		return 0, errors.New("submodelElement is not of type Property")
	}

	// Create the nested property with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement)
	if err != nil {
		return 0, err
	}

	// Property-specific database insertion for nested element
	err = insertProperty(property, err, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLPropertyHandler) Read(idShortOrPath string) error {
	if dErr := p.decorated.Read(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLPropertyHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLPropertyHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertProperty(property *gen.Property, err error, tx *sql.Tx, id int) error {
	var valueText, valueNum, valueBool, valueTime, valueDatetime sql.NullString
	var valueId sql.NullInt64

	switch property.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		valueText = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		valueNum = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:boolean":
		valueBool = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:time":
		valueTime = sql.NullString{String: property.Value, Valid: property.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		valueDatetime = sql.NullString{String: property.Value, Valid: property.Value != ""}
	default:
		// Fallback to text for unknown types
		valueText = sql.NullString{String: property.Value, Valid: property.Value != ""}
	}

	// Handle valueId if present
	if len(property.ValueId.Keys) > 0 && property.ValueId.Keys[0].Value != "" {
		// Assuming ValueId references another element by ID - you may need to adjust this logic
		valueId = sql.NullInt64{Int64: 0, Valid: false} // Implement proper ID resolution here
	}

	// Insert Property-specific data
	_, err = tx.Exec(`INSERT INTO property_element (id, value_type, value_text, value_num, value_bool, value_time, value_datetime, value_id)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		id,
		property.ValueType,
		valueText,
		valueNum,
		valueBool,
		valueTime,
		valueDatetime,
		valueId,
	)
	return err
}
