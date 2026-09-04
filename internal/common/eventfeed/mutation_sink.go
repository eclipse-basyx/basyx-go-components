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

package eventfeed

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
)

const (
	mutationTableAAS      = "aas_history"
	mutationTableSubmodel = "submodel_history"
	mutationCreated       = "Created"
	mutationUpdated       = "Updated"
	mutationDeleted       = "Deleted"
)

// Mutation is one model change observed inside the writer transaction.
type Mutation struct {
	Table            string
	Identifier       string
	ChangeType       string
	PreviousSnapshot map[string]any
	Snapshot         map[string]any
	Deleted          bool
}

// MutationSink writes CloudEvents feed rows inside the authoritative mutation transaction.
type MutationSink struct {
	service *Service
}

// NewMutationSink creates a mutation sink backed by svc.
func NewMutationSink(svc *Service) *MutationSink {
	return &MutationSink{service: svc}
}

// HandleMutation persists feed events for mutation.
func (s *MutationSink) HandleMutation(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	if s == nil || s.service == nil || !s.service.cfg.Enabled || tx == nil {
		return nil
	}
	switch mutation.Table {
	case mutationTableAAS:
		return s.handleAAS(ctx, tx, mutation)
	case mutationTableSubmodel:
		return s.handleSubmodel(ctx, tx, mutation)
	default:
		return nil
	}
}

func (s *MutationSink) handleAAS(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	snap := mutation.Snapshot
	if mutation.Deleted {
		snap = mutation.PreviousSnapshot
		if snap == nil {
			snap = mutation.Snapshot
		}
	}
	aasID, globalAssetID, submodels := aasFieldsFromSnapshot(snap)
	if aasID == "" {
		aasID = mutation.Identifier
	}

	var buildFn func(string, string, []SubmodelRef) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		buildFn = s.service.build.AASCreated
	case mutationDeleted:
		buildFn = s.service.build.AASDeleted
	default:
		buildFn = s.service.build.AASUpdated
	}
	ev, err := buildFn(aasID, globalAssetID, submodels)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-AAS-BUILD: %w", err)
	}
	if err = s.service.WriteTx(ctx, tx, ev); err != nil {
		return err
	}
	if globalAssetID == "" {
		return nil
	}
	var assetFn func(string, string, []SubmodelRef) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		assetFn = s.service.build.AssetCreated
	case mutationDeleted:
		assetFn = s.service.build.AssetDeleted
	default:
		assetFn = s.service.build.AssetUpdated
	}
	aev, err := assetFn(globalAssetID, aasID, submodels)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-ASSET-BUILD: %w", err)
	}
	return s.service.WriteTx(ctx, tx, aev)
}

func (s *MutationSink) handleSubmodel(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	snap := mutation.Snapshot
	if mutation.Deleted {
		snap = mutation.PreviousSnapshot
		if snap == nil {
			snap = mutation.Snapshot
		}
	}
	submodelID, semanticID := submodelFieldsFromSnapshot(snap)
	if submodelID == "" {
		submodelID = mutation.Identifier
	}
	globalAssetIDs, err := globalAssetIDsForSubmodelTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	var buildFn func(string, string, []string) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		buildFn = s.service.build.SubmodelCreated
	case mutationDeleted:
		buildFn = s.service.build.SubmodelDeleted
	default:
		buildFn = s.service.build.SubmodelUpdated
	}
	ev, err := buildFn(submodelID, semanticID, globalAssetIDs)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-SUBMODEL-BUILD: %w", err)
	}
	if err = s.service.WriteTx(ctx, tx, ev); err != nil {
		return err
	}

	if !IsPCNSemanticID(semanticID) || mutation.Deleted {
		return nil
	}

	currentSubmodel, err := submodelFromSnapshot(snap)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-PCN-DESERIALIZE: %w", err)
	}
	previousSubmodel, err := submodelFromSnapshot(mutation.PreviousSnapshot)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-PCN-DESERIALIZE: %w", err)
	}
	for _, record := range PCNNewRecordValuesFromSubmodel(previousSubmodel, currentSubmodel) {
		pcnEv, pcnErr := s.service.build.PCN(submodelID, globalAssetIDs, record)
		if pcnErr != nil {
			return fmt.Errorf("EVENTFEED-MUTATION-PCN-BUILD: %w", pcnErr)
		}
		if err = s.service.WriteTx(ctx, tx, pcnEv); err != nil {
			return err
		}
	}
	return nil
}

// submodelFromSnapshot deserializes a full submodel JSON snapshot back into
// its typed representation so PCN records can be converted to Value-Only
// via the shared model helpers.
func submodelFromSnapshot(snap map[string]any) (types.ISubmodel, error) {
	if snap == nil {
		return nil, nil
	}
	return jsonization.SubmodelFromJsonable(snap)
}
