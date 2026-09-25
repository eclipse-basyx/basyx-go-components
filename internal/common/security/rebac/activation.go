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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package rebac

import (
	"context"
	"time"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/rebac/model"
)

const activationTable = "rebac_model_activation"

// Activation binds the database to one scope, store and pinned model.
type Activation struct {
	Scope       string    `json:"scope"`
	StoreID     string    `json:"storeId"`
	ModelID     string    `json:"authorizationModelId"`
	ModelHash   string    `json:"modelHash"`
	ActivatedBy string    `json:"activatedBy"`
	ActivatedAt time.Time `json:"activatedAt"`
}

// ReadActivation returns the binding of the database, if any.
func ReadActivation(ctx context.Context, q Queryer) (Activation, bool, error) {
	ds := dialect.From(goqu.T(activationTable)).Select(
		goqu.C("scope"), goqu.C("store_id"), goqu.C("model_id"), goqu.C("model_hash"),
		goqu.C("activated_by"), goqu.C("activated_at"),
	).Limit(1).Prepared(true)
	var activation Activation
	found, err := queryRowDataset(ctx, q, "REBAC-READACTIVATION", ds,
		&activation.Scope, &activation.StoreID, &activation.ModelID, &activation.ModelHash,
		&activation.ActivatedBy, &activation.ActivatedAt)
	return activation, found, err
}

// InsertActivation binds the database unless it is already bound.
func InsertActivation(ctx context.Context, q Queryer, activation Activation) error {
	ds := dialect.Insert(activationTable).Rows(goqu.Record{
		"scope": activation.Scope, "store_id": activation.StoreID, "model_id": activation.ModelID,
		"model_hash": activation.ModelHash, "activated_by": activation.ActivatedBy,
	}).OnConflict(goqu.DoNothing()).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-INSERTACTIVATION", ds)
	return err
}

// ActivateModel records a newly provisioned model for the bound scope.
func ActivateModel(ctx context.Context, q Queryer, activation Activation) error {
	ds := dialect.Update(activationTable).Set(goqu.Record{
		"store_id": activation.StoreID, "model_id": activation.ModelID, "model_hash": activation.ModelHash,
		"activated_by": activation.ActivatedBy, "activated_at": goqu.L("clock_timestamp()"),
	}).Where(goqu.C("scope").Eq(activation.Scope)).Prepared(true)
	_, err := execDataset(ctx, q, "REBAC-ACTIVATEMODEL", ds)
	return err
}

// ModelContentHash hashes the semantic content of a model as embedded or as
// returned by OpenFGA.
func ModelContentHash(raw []byte) (string, error) {
	return model.CanonicalHash(raw)
}

// EmbeddedModelHash returns the content hash of the model of this release.
func EmbeddedModelHash() (string, error) {
	return ModelContentHash(model.JSON())
}
