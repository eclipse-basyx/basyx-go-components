package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLRangeHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLRangeHandler(db *sql.DB) (*PostgreSQLRangeHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLRangeHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLRangeHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, errors.New("submodelElement is not of type Range")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLRangeHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	rangeElem, ok := submodelElement.(*gen.Range)
	if !ok {
		return 0, errors.New("submodelElement is not of type Range")
	}

	// Create the nested range with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// Range-specific database insertion for nested element
	err = insertRange(rangeElem, tx, id)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLRangeHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.Range{}
	var valueType, min, max string
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`
		SELECT value_type, COALESCE(min_text, min_num::text, min_time::text, min_datetime::text) as min_val,
		COALESCE(max_text, max_num::text, max_time::text, max_datetime::text) as max_val
		FROM range_element
		WHERE id = $1
	`, id).Scan(&valueType, &min, &max)
	if err != nil {
		return sme, nil // Return base if no specific data
	}
	rng := sme.(*gen.Range)
	actualValueType, err := gen.NewDataTypeDefXsdFromValue(valueType)
	if err != nil {
		return nil, err
	}
	rng.ValueType = actualValueType
	rng.Min = min
	rng.Max = max
	return sme, nil
}
func (p PostgreSQLRangeHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLRangeHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertRange(rangeElem *gen.Range, tx *sql.Tx, id int) error {
	var minText, maxText, minNum, maxNum, minTime, maxTime, minDatetime, maxDatetime sql.NullString

	switch rangeElem.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		minText = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxText = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		minNum = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxNum = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:time":
		minTime = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxTime = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		minDatetime = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxDatetime = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	default:
		// Fallback to text
		minText = sql.NullString{String: rangeElem.Min, Valid: rangeElem.Min != ""}
		maxText = sql.NullString{String: rangeElem.Max, Valid: rangeElem.Max != ""}
	}

	// Insert Range-specific data
	_, err := tx.Exec(`INSERT INTO range_element (id, value_type, min_text, max_text, min_num, max_num, min_time, max_time, min_datetime, max_datetime)
					 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, rangeElem.ValueType,
		minText, maxText, minNum, maxNum, minTime, maxTime, minDatetime, maxDatetime)
	return err
}
