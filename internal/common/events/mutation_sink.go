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

package events

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
	Acknowledged     bool
}

// MutationSink generates CloudEvents inside the authoritative model transaction.
type MutationSink struct {
	build *Builder
	write EventWriter
}

// NewMutationSink creates a transaction-scoped event consumer.
//
// Parameters:
//   - builder: Builder used once per generated event.
//   - write: Writer receiving the captured event and original model transaction.
//
// Returns:
//   - *MutationSink: Consumer; a nil builder or writer produces a disabled consumer.
func NewMutationSink(builder *Builder, write EventWriter) *MutationSink {
	return &MutationSink{build: builder, write: write}
}

// EventWriter persists a captured event using the supplied request context and
// model transaction. mutation identifies the ordering domain. Writers must not
// commit tx or modify event; an error causes the model mutation to roll back.
type EventWriter func(context.Context, *sql.Tx, Mutation, FeedEvent) error

// HandleMutation generates and writes events for a captured model mutation.
//
// The caller must hold the entity mutation lock and roll back tx on error.
// Acknowledged no-op writes and unsupported history tables produce no events.
//
// Parameters:
//   - ctx: Original request context, including security metadata.
//   - tx: Active model transaction owned by the caller.
//   - mutation: Entity identity and complete snapshots before and after the write.
//
// Returns:
//   - error: First construction or write error; nil on success, a disabled consumer, or a skipped mutation.
func (s *MutationSink) HandleMutation(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	if s == nil || s.build == nil || s.write == nil || tx == nil {
		return nil
	}
	if mutation.Acknowledged {
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

func mutationSnapshot(mutation Mutation) map[string]any {
	if mutation.Deleted && mutation.PreviousSnapshot != nil {
		return mutation.PreviousSnapshot
	}
	return mutation.Snapshot
}

func (s *MutationSink) handleAAS(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	snap := mutationSnapshot(mutation)
	aasID, globalAssetID, submodels := AASFieldsFromSnapshot(snap)
	if aasID == "" {
		aasID = mutation.Identifier
	}

	var buildFn func(string, string, []SubmodelRef) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		buildFn = s.build.AASCreated
	case mutationDeleted:
		buildFn = s.build.AASDeleted
	default:
		buildFn = s.build.AASUpdated
	}
	ev, err := buildFn(aasID, globalAssetID, submodels)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-AAS-BUILD: %w", err)
	}
	if err = s.write(ctx, tx, mutation, ev); err != nil {
		return err
	}
	if globalAssetID == "" {
		return nil
	}
	var assetFn func(string, string, []SubmodelRef) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		assetFn = s.build.AssetCreated
	case mutationDeleted:
		assetFn = s.build.AssetDeleted
	default:
		assetFn = s.build.AssetUpdated
	}
	aev, err := assetFn(globalAssetID, aasID, submodels)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-ASSET-BUILD: %w", err)
	}
	return s.write(ctx, tx, mutation, aev)
}

func (s *MutationSink) handleSubmodel(ctx context.Context, tx *sql.Tx, mutation Mutation) error {
	snap := mutationSnapshot(mutation)
	submodelID, semanticID := submodelFieldsFromSnapshot(snap)
	if submodelID == "" {
		submodelID = mutation.Identifier
	}
	globalAssetIDs, aasIDs, err := submodelAssetOwnersTx(ctx, tx, submodelID)
	if err != nil {
		return err
	}

	var buildFn func(string, string, []string) (FeedEvent, error)
	switch mutation.ChangeType {
	case mutationCreated:
		buildFn = s.build.SubmodelCreated
	case mutationDeleted:
		buildFn = s.build.SubmodelDeleted
	default:
		buildFn = s.build.SubmodelUpdated
	}
	ev, err := buildFn(submodelID, semanticID, globalAssetIDs)
	if err != nil {
		return fmt.Errorf("EVENTFEED-MUTATION-SUBMODEL-BUILD: %w", err)
	}
	ev.AuthorizationAASIDs = aasIDs
	if err = s.write(ctx, tx, mutation, ev); err != nil {
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
		pcnEv, pcnErr := s.build.PCN(submodelID, globalAssetIDs, record)
		if pcnErr != nil {
			return fmt.Errorf("EVENTFEED-MUTATION-PCN-BUILD: %w", pcnErr)
		}
		pcnEv.AuthorizationAASIDs = aasIDs
		if err = s.write(ctx, tx, mutation, pcnEv); err != nil {
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
