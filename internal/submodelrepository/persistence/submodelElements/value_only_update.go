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
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/FriedJannik/aas-go-sdk/stringification"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	jsoniter "github.com/json-iterator/go"
)

// ValueOnlyUpdater applies a decoded ValueOnly value to a stored submodel element of one model type.
// The update carries the element's database id, so implementations write by id without path lookups.
type ValueOnlyUpdater interface {
	UpdateValueOnly(tx *sql.Tx, update gen.ValueOnlyUpdate) error
}

// ApplyValueOnlyUpdates applies decoded ValueOnly updates in order within the given transaction.
func ApplyValueOnlyUpdates(tx *sql.Tx, db *sql.DB, updates []gen.ValueOnlyUpdate) error {
	updaters := make(map[types.ModelType]ValueOnlyUpdater)
	for _, update := range updates {
		updater, err := valueOnlyUpdaterFor(update.ModelType, db, updaters)
		if err != nil {
			return err
		}
		if err = updater.UpdateValueOnly(tx, update); err != nil {
			return err
		}
	}
	return nil
}

func valueOnlyUpdaterFor(modelType types.ModelType, db *sql.DB, cache map[types.ModelType]ValueOnlyUpdater) (ValueOnlyUpdater, error) {
	if updater, ok := cache[modelType]; ok {
		return updater, nil
	}
	handler, err := GetSMEHandlerByModelType(modelType, db)
	if err != nil {
		return nil, err
	}
	updater, ok := handler.(ValueOnlyUpdater)
	if !ok {
		name, _ := stringification.ModelTypeToString(modelType)
		return nil, common.NewErrBadRequest("SMREPO-APPLYVALONLY-UNSUPPORTED " + name + " has no ValueOnly representation")
	}
	cache[modelType] = updater
	return updater, nil
}

func valueOnlyPatchAs[T gen.ValueOnlyPatch](update gen.ValueOnlyUpdate) (T, error) {
	patch, ok := update.Value.(T)
	if !ok {
		var zero T
		return zero, common.NewInternalServerError(fmt.Sprintf("SMREPO-UPDVALONLY-PATCHTYPE unexpected ValueOnly value %T for %s", update.Value, update.IDShortPath))
	}
	return patch, nil
}

func execValueOnlyUpdate(tx *sql.Tx, table string, record goqu.Record, elementID int64, errorCode string) error {
	if len(record) == 0 {
		return nil
	}
	query, args, err := goqu.Dialect("postgres").
		Update(table).
		Set(record).
		Where(goqu.C("id").Eq(elementID)).
		Prepared(true).
		ToSQL()
	if err != nil {
		return common.NewInternalServerError(errorCode + "-BUILDQ " + err.Error())
	}
	if _, err = tx.Exec(query, args...); err != nil {
		slog.Error("value-only update failed", "error.code", errorCode+"-EXECQ", "table", table, "error", err)
		return common.NewInternalServerError(errorCode + "-EXECQ failed to update " + table)
	}
	return nil
}

func valueOnlyReferenceColumn(reference types.IReference) (any, error) {
	return serializeReference(reference, jsoniter.ConfigCompatibleWithStandardLibrary)
}

func addValueOnlyReferenceColumn(record goqu.Record, column string, reference types.IReference) error {
	if reference == nil {
		return nil
	}
	value, err := valueOnlyReferenceColumn(reference)
	if err != nil {
		return err
	}
	record[column] = value
	return nil
}

func updateValueOnlyRelationship(tx *sql.Tx, table string, update gen.ValueOnlyUpdate, errorCode string) error {
	patch, err := valueOnlyPatchAs[gen.RelationshipValuePatch](update)
	if err != nil {
		return err
	}
	record := goqu.Record{}
	if err = addValueOnlyReferenceColumn(record, "first", patch.First); err != nil {
		return err
	}
	if err = addValueOnlyReferenceColumn(record, "second", patch.Second); err != nil {
		return err
	}
	return execValueOnlyUpdate(tx, table, record, update.ElementID, errorCode)
}
