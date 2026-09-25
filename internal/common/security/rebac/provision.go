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
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/rebac/model"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
)

const provisionTimeout = 30 * time.Second

// StoreName returns the OpenFGA store name of a scope.
func StoreName(scope string) string {
	return "basyx-" + scope
}

// Provision creates or finds the store of the scope, writes the embedded
// model unless a model with the same content already exists, and binds the
// database to both. It is run by basyxconfigurationservice after the schema
// patches when rebac.provisionModel is enabled.
func Provision(ctx context.Context, cfg common.ReBACConfig, db *sql.DB, actor string) (Activation, error) {
	ctx, cancel := context.WithTimeout(ctx, provisionTimeout)
	defer cancel()
	current, bound, err := ReadActivation(ctx, db)
	if err != nil {
		return Activation{}, err
	}
	if bound && current.Scope != cfg.Scope {
		return Activation{}, fmt.Errorf("REBAC-PROVISION-SCOPEMISMATCH database is bound to scope %q", current.Scope)
	}
	sdk, err := newSDKClient(cfg.OpenFGA, "", "")
	if err != nil {
		return Activation{}, err
	}
	storeID, err := provisionStore(ctx, sdk, cfg, current, bound)
	if err != nil {
		return Activation{}, err
	}
	hash, err := EmbeddedModelHash()
	if err != nil {
		return Activation{}, err
	}
	modelID, err := provisionModel(ctx, sdk, storeID, hash)
	if err != nil {
		return Activation{}, err
	}
	activation := Activation{Scope: cfg.Scope, StoreID: storeID, ModelID: modelID, ModelHash: hash, ActivatedBy: actor}
	if bound {
		err = ActivateModel(ctx, db, activation)
	} else {
		err = InsertActivation(ctx, db, activation)
	}
	if err != nil {
		return Activation{}, err
	}
	return activation, EnsureScopeState(ctx, db, cfg.Scope)
}

func provisionStore(ctx context.Context, sdk *fgaclient.OpenFgaClient, cfg common.ReBACConfig, current Activation, bound bool) (string, error) {
	if cfg.OpenFGA.StoreID != "" {
		if bound && current.StoreID != cfg.OpenFGA.StoreID {
			return "", fmt.Errorf("REBAC-PROVISION-STOREMISMATCH database is bound to store %q", current.StoreID)
		}
		return cfg.OpenFGA.StoreID, nil
	}
	if bound {
		return current.StoreID, nil
	}
	name := StoreName(cfg.Scope)
	listed, err := sdk.ListStores(ctx).Options(fgaclient.ClientListStoresOptions{Name: &name}).Execute()
	if err != nil {
		return "", fmt.Errorf("REBAC-PROVISION-LISTSTORES: %w", err)
	}
	if stores := listed.GetStores(); len(stores) > 0 {
		return stores[0].GetId(), nil
	}
	created, err := sdk.CreateStore(ctx).Body(fgaclient.ClientCreateStoreRequest{Name: name}).Execute()
	if err != nil {
		return "", fmt.Errorf("REBAC-PROVISION-CREATESTORE: %w", err)
	}
	return created.GetId(), nil
}

func provisionModel(ctx context.Context, sdk *fgaclient.OpenFgaClient, storeID string, hash string) (string, error) {
	existing, err := findModelByHash(ctx, sdk, storeID, hash)
	if err != nil || existing != "" {
		return existing, err
	}
	var body fgasdk.WriteAuthorizationModelRequest
	if err = json.Unmarshal(model.JSON(), &body); err != nil {
		return "", fmt.Errorf("REBAC-PROVISION-PARSEMODEL: %w", err)
	}
	written, err := sdk.WriteAuthorizationModel(ctx).Body(body).
		Options(fgaclient.ClientWriteAuthorizationModelOptions{StoreId: &storeID}).Execute()
	if err != nil {
		return "", fmt.Errorf("REBAC-PROVISION-WRITEMODEL: %w", err)
	}
	return written.GetAuthorizationModelId(), nil
}

func findModelByHash(ctx context.Context, sdk *fgaclient.OpenFgaClient, storeID string, hash string) (string, error) {
	continuation := ""
	for {
		options := fgaclient.ClientReadAuthorizationModelsOptions{StoreId: &storeID}
		if continuation != "" {
			options.ContinuationToken = &continuation
		}
		response, err := sdk.ReadAuthorizationModels(ctx).Options(options).Execute()
		if err != nil {
			return "", fmt.Errorf("REBAC-PROVISION-READMODELS: %w", err)
		}
		modelID, err := modelWithHash(response.GetAuthorizationModels(), hash)
		if err != nil || modelID != "" {
			return modelID, err
		}
		continuation = response.GetContinuationToken()
		if continuation == "" {
			return "", nil
		}
	}
}

func modelWithHash(models []fgasdk.AuthorizationModel, hash string) (string, error) {
	for _, candidate := range models {
		raw, err := json.Marshal(candidate)
		if err != nil {
			return "", fmt.Errorf("REBAC-PROVISION-ENCODEMODEL: %w", err)
		}
		candidateHash, err := ModelContentHash(raw)
		if err != nil {
			return "", err
		}
		if candidateHash == hash {
			return candidate.GetId(), nil
		}
	}
	return "", nil
}
