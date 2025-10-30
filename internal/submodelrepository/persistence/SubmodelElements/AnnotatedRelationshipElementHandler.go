/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Author: Jannik Fried ( Fraunhofer IESE )
package submodelelements

import (
	"database/sql"
	"errors"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLAnnotatedRelationshipElementHandler struct {
	db        *sql.DB
	decorated *PostgreSQLSMECrudHandler
}

func NewPostgreSQLAnnotatedRelationshipElementHandler(db *sql.DB) (*PostgreSQLAnnotatedRelationshipElementHandler, error) {
	decoratedHandler, err := NewPostgreSQLSMECrudHandler(db)
	if err != nil {
		return nil, err
	}
	return &PostgreSQLAnnotatedRelationshipElementHandler{db: db, decorated: decoratedHandler}, nil
}

func (p PostgreSQLAnnotatedRelationshipElementHandler) Create(tx *sql.Tx, submodelId string, submodelElement gen.SubmodelElement) (int, error) {
	areElem, ok := submodelElement.(*gen.AnnotatedRelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type AnnotatedRelationshipElement")
	}

	// First, perform base SubmodelElement operations within the transaction
	id, err := p.decorated.Create(tx, submodelId, submodelElement)
	if err != nil {
		return 0, err
	}

	// AnnotatedRelationshipElement-specific database insertion
	err = insertAnnotatedRelationshipElement(areElem, tx, id, submodelId, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLAnnotatedRelationshipElementHandler) CreateNested(tx *sql.Tx, submodelId string, parentId int, idShortPath string, submodelElement gen.SubmodelElement, pos int) (int, error) {
	areElem, ok := submodelElement.(*gen.AnnotatedRelationshipElement)
	if !ok {
		return 0, errors.New("submodelElement is not of type AnnotatedRelationshipElement")
	}

	// Create the nested areElem with the provided idShortPath using the decorated handler
	id, err := p.decorated.CreateAndPath(tx, submodelId, parentId, idShortPath, submodelElement, pos)
	if err != nil {
		return 0, err
	}

	// AnnotatedRelationshipElement-specific database insertion for nested element
	err = insertAnnotatedRelationshipElement(areElem, tx, id, submodelId, p.db)
	if err != nil {
		return 0, err
	}

	return id, nil
}

func (p PostgreSQLAnnotatedRelationshipElementHandler) Read(tx *sql.Tx, submodelId string, idShortOrPath string) (gen.SubmodelElement, error) {
	var sme gen.SubmodelElement = &gen.AnnotatedRelationshipElement{}
	var firstRef, secondRef sql.NullInt64
	id, err := p.decorated.Read(tx, submodelId, idShortOrPath, &sme)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(`SELECT first_ref, second_ref FROM relationship_element WHERE id = $1`, id).Scan(&firstRef, &secondRef)
	if err != nil {
		return sme, nil
	}
	areElem := sme.(*gen.AnnotatedRelationshipElement)
	if firstRef.Valid {
		ref, err := readReference(tx, firstRef.Int64)
		if err != nil {
			return nil, err
		}
		areElem.First = ref
	}
	if secondRef.Valid {
		ref, err := readReference(tx, secondRef.Int64)
		if err != nil {
			return nil, err
		}
		areElem.Second = ref
	}

	// Read annotations
	rows, err := tx.Query(`SELECT annotation_sme FROM annotated_rel_annotation WHERE rel_id = $1`, id)
	if err != nil {
		return sme, nil
	}
	defer rows.Close()
	var annotations []gen.SubmodelElement
	for rows.Next() {
		var annId int
		if err := rows.Scan(&annId); err != nil {
			return nil, err
		}
		// Read the annotation element
		// But need to know the type. Perhaps query the model_type from submodel_element
		var modelType string
		err = tx.QueryRow(`SELECT model_type FROM submodel_element WHERE id = $1`, annId).Scan(&modelType)
		if err != nil {
			return nil, err
		}
		annHandler, err := GetSMEHandlerByModelType(modelType, p.db)
		if err != nil {
			return nil, err
		}
		// But Read needs idShortOrPath, but we have id.
		// Need to get idShortPath
		var idShortPath string
		err = tx.QueryRow(`SELECT idshort_path FROM submodel_element WHERE id = $1`, annId).Scan(&idShortPath)
		if err != nil {
			return nil, err
		}
		ann, err := annHandler.Read(tx, submodelId, idShortPath)
		if err != nil {
			return nil, err
		}
		annotations = append(annotations, ann)
	}
	areElem.Annotations = annotations
	return sme, nil
}
func (p PostgreSQLAnnotatedRelationshipElementHandler) Update(idShortOrPath string, submodelElement gen.SubmodelElement) error {
	if dErr := p.decorated.Update(idShortOrPath, submodelElement); dErr != nil {
		return dErr
	}
	return nil
}
func (p PostgreSQLAnnotatedRelationshipElementHandler) Delete(idShortOrPath string) error {
	if dErr := p.decorated.Delete(idShortOrPath); dErr != nil {
		return dErr
	}
	return nil
}

func insertAnnotatedRelationshipElement(areElem *gen.AnnotatedRelationshipElement, tx *sql.Tx, id int, submodelId string, db *sql.DB) error {
	// Insert into relationship_element
	var firstRefId, secondRefId sql.NullInt64

	if !isEmptyReference(areElem.First) {
		refId, err := insertReference(tx, *areElem.First)
		if err != nil {
			return err
		}
		firstRefId = sql.NullInt64{Int64: int64(refId), Valid: true}
	}

	if !isEmptyReference(areElem.Second) {
		refId, err := insertReference(tx, *areElem.Second)
		if err != nil {
			return err
		}
		secondRefId = sql.NullInt64{Int64: int64(refId), Valid: true}
	}

	_, err := tx.Exec(`INSERT INTO relationship_element (id, first_ref, second_ref) VALUES ($1, $2, $3)`,
		id, firstRefId, secondRefId)
	if err != nil {
		return err
	}

	// Create annotations as separate submodel elements
	for _, annotation := range areElem.Annotations {
		annHandler, err := GetSMEHandler(annotation, db)
		if err != nil {
			return err
		}

		annId, err := annHandler.Create(tx, submodelId, annotation)
		if err != nil {
			return err
		}

		// Insert link
		_, err = tx.Exec(`INSERT INTO annotated_rel_annotation (rel_id, annotation_sme) VALUES ($1, $2)`, id, annId)
		if err != nil {
			return err
		}
	}

	return nil
}
