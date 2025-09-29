package persistence_utils

import (
	"database/sql"
	"reflect"

	gen "github.com/eclipse-basyx/basyx-go-components/pkg/submodelrepositoryapi/go"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

func CreateSemanticId(tx *sql.Tx, semanticId *gen.Reference) (sql.NullInt64, error) {
	var id int
	var referenceID sql.NullInt64
	if semanticId != nil && !isEmptyReference(*semanticId) {
		err := tx.QueryRow(`INSERT INTO reference (type) VALUES ($1) RETURNING id`, semanticId.Type).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		referenceID = sql.NullInt64{Int64: int64(id), Valid: true}

		references := semanticId.Keys
		for i := range references {
			_, err = tx.Exec(`INSERT INTO reference_key (reference_id, position, type, value) VALUES ($1, $2, $3, $4)`,
				id, i, references[i].Type, references[i].Value)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}
	}
	return referenceID, nil
}

func GetSemanticId(db *sql.DB, referenceID sql.NullInt64) (*gen.Reference, error) {
	if !referenceID.Valid {
		return nil, nil
	}
	var refType string
	// avoid driver-specific type casts in the query string which can confuse the pq parser
	print(referenceID.Int64)
	err := db.QueryRow(`SELECT type FROM reference WHERE id=$1`, referenceID.Int64).Scan(&refType)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	// similarly, select the type column directly and let the driver handle conversion
	rows, err := db.Query(`SELECT type, value FROM reference_key WHERE reference_id=$1 ORDER BY position`, referenceID.Int64)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []gen.Key
	for rows.Next() {
		var keyType, value string
		if err := rows.Scan(&keyType, &value); err != nil {
			return nil, err
		}
		cKeyType, err := gen.NewKeyTypesFromValue(keyType)
		if err != nil {
			return nil, err
		}
		keys = append(keys, gen.Key{Type: cKeyType, Value: value})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cRefType, err := gen.NewReferenceTypesFromValue(refType)
	if err != nil {
		return nil, err
	}
	return &gen.Reference{
		Type: gen.ReferenceTypes(cRefType),
		Keys: keys,
	}, nil
}

func CreateLangStringNameTypes(tx *sql.Tx, nameTypes []*gen.LangStringNameType) (sql.NullInt64, error) {
	var id int
	var nameTypeID sql.NullInt64
	if nameTypes != nil {
		err := tx.QueryRow(`INSERT INTO lang_string_name_type_reference RETURNING id`).Scan(&id)
		if err != nil {
			return sql.NullInt64{}, err
		}
		nameTypeID = sql.NullInt64{Int64: int64(id), Valid: true}
		for i := 1; i < len(nameTypes); i++ {
			err := tx.QueryRow(`INSERT INTO lang_string_name_type (lang_string_name_type_reference_id, text, language) VALUES ($1, $2, $3) RETURNING id`, nameTypeID.Int64, nameTypes[i].Text, nameTypes[i].Language).Scan(&id)
			if err != nil {
				return sql.NullInt64{}, err
			}
		}
	}
	return nameTypeID, nil
}

func GetLangStringNameTypes(db *sql.DB, nameTypeID sql.NullInt64) ([]*gen.LangStringNameType, error) {
	if !nameTypeID.Valid {
		return nil, nil
	}
	var nameTypes []*gen.LangStringNameType
	rows, err := db.Query(`SELECT text, language FROM lang_string_name_type WHERE lang_string_name_type_reference_id=$1`, nameTypeID.Int64)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var text, language string
		if err := rows.Scan(&text, &language); err != nil {
			return nil, err
		}
		nameTypes = append(nameTypes, &gen.LangStringNameType{Text: text, Language: language})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nameTypes, nil
}

// isEmptyReference checks if a Reference is empty (zero value)

func isEmptyReference(ref gen.Reference) bool {
	return reflect.DeepEqual(ref, gen.Reference{})
}
