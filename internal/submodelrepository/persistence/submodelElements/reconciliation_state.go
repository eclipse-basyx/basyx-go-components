/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package submodelelements

import (
	"context"
	"database/sql"
	"errors"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ReconciliationParent describes a persisted container element whose children
// are reconciled.
type ReconciliationParent struct {
	Path      string
	Depth     int
	ModelType types.ModelType
}

// ChildInsertContext returns the placement of the parent's direct children.
func (p ReconciliationParent) ChildInsertContext() *BatchInsertContext {
	return &BatchInsertContext{
		ParentPath: p.Path,
		IsFromList: p.ModelType == types.ModelTypeSubmodelElementList,
		StartDepth: p.Depth + 1,
	}
}

// ChildElements returns the direct children of a container element, or nil
// for elements without children.
func ChildElements(element types.ISubmodelElement) []types.ISubmodelElement {
	return getChildElements(element)
}

// ProvidedChildElements returns the children carried by a container element and
// whether the element carries a children field at all. A nil field means that
// the children are not part of the request.
func ProvidedChildElements(element types.ISubmodelElement) ([]types.ISubmodelElement, bool) {
	switch typed := element.(type) {
	case *types.SubmodelElementCollection:
		return typed.Value(), typed.Value() != nil
	case *types.SubmodelElementList:
		return typed.Value(), typed.Value() != nil
	case *types.Entity:
		return typed.Statements(), typed.Statements() != nil
	case *types.AnnotatedRelationshipElement:
		return getChildElements(typed), typed.Annotations() != nil
	default:
		return nil, false
	}
}

// IsContainerModelType reports whether elements of the model type own child elements.
func IsContainerModelType(modelType types.ModelType) bool {
	switch modelType {
	case types.ModelTypeSubmodelElementCollection, types.ModelTypeSubmodelElementList,
		types.ModelTypeEntity, types.ModelTypeAnnotatedRelationshipElement:
		return true
	default:
		return false
	}
}

// LoadReconciliationParentTx reads the persisted placement of a container element.
func LoadReconciliationParentTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, path string) (ReconciliationParent, error) {
	query, args, err := goqu.Dialect(common.Dialect).From("submodel_element").
		Select("depth", "model_type").
		Where(
			goqu.C("submodel_id").Eq(submodelDatabaseID),
			goqu.C("idshort_path").Eq(path),
		).
		Prepared(true).
		ToSQL()
	if err != nil {
		return ReconciliationParent{}, common.NewInternalServerError("SMREPO-RECONPARENT-BUILDQ " + err.Error())
	}
	parent := ReconciliationParent{Path: path}
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&parent.Depth, &parent.ModelType); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ReconciliationParent{}, common.NewErrNotFound("SMREPO-RECONPARENT-NOTFOUND Submodel-Element ID-Short: " + path)
		}
		return ReconciliationParent{}, common.NewInternalServerError("SMREPO-RECONPARENT-EXECQ " + err.Error())
	}
	return parent, nil
}

// LoadPersistedPositionsTx returns the stored sibling positions of all
// elements below parentPath, or of all elements of the Submodel when
// parentPath is empty.
func LoadPersistedPositionsTx(ctx context.Context, tx *sql.Tx, submodelDatabaseID int, parentPath string) (map[string]int, error) {
	var scope goqu.Expression = goqu.C("submodel_id").Eq(submodelDatabaseID)
	if parentPath != "" {
		scope = submodelElementTreeWhere(submodelDatabaseID, parentPath, false, "")
	}
	query, args, err := goqu.Dialect(common.Dialect).From("submodel_element").
		Select("idshort_path", "position").
		Where(scope).
		Prepared(true).
		ToSQL()
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-RECONPOS-BUILDQ " + err.Error())
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, common.NewInternalServerError("SMREPO-RECONPOS-EXECQ " + err.Error())
	}
	defer func() { _ = rows.Close() }()
	positions := make(map[string]int)
	for rows.Next() {
		var path string
		var position int
		if err = rows.Scan(&path, &position); err != nil {
			return nil, common.NewInternalServerError("SMREPO-RECONPOS-SCANQ " + err.Error())
		}
		positions[path] = position
	}
	if err = rows.Err(); err != nil {
		return nil, common.NewInternalServerError("SMREPO-RECONPOS-ROWSERR " + err.Error())
	}
	return positions, nil
}

// AlignPersistedPositions replaces the positions derived from the element
// order by the stored positions. Deletions leave gaps in stored positions,
// which must be detected as position changes to keep sibling positions unique.
func AlignPersistedPositions(rows []ReconciliationElementRow, positions map[string]int) {
	for index := range rows {
		if position, exists := positions[rows[index].Path]; exists {
			rows[index].Position = position
		}
	}
}
