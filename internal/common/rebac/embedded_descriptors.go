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

package rebac

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/doug-martin/goqu/v9"
)

// EmbeddedDescriptorIdentifier separates an embedding's identity from a standalone descriptor.
func EmbeddedDescriptorIdentifier(aasID, submodelID string) string {
	return ElementIdentifier(aasID, submodelID)
}

// SyncEmbeddedDescriptors preserves independent generations and grants for embedded descriptors.
func (r *Runtime) SyncEmbeddedDescriptors(ctx context.Context, tx *sql.Tx, parent StoredResource, creator string, deleted bool) ([]Tuple, []Tuple, error) {
	ids := []string{}
	if !deleted {
		var err error
		ids, err = embeddedDescriptorIDs(ctx, tx, parent.Identifier)
		if err != nil {
			return nil, nil, err
		}
	}
	live := map[string]bool{}
	var writes []Tuple
	for _, id := range ids {
		identity := EmbeddedDescriptorIdentifier(parent.Identifier, id)
		live[identity] = true
		childCreator := creator
		if r.Creator != nil {
			childCreator = r.Creator(ctx, "embedded_submodel_descriptor", identity, creator)
		}
		_, tuples, err := r.State.EnsureResource(ctx, tx, "embedded_submodel_descriptor", identity, parent.UUID, childCreator)
		if err != nil {
			return nil, nil, err
		}
		writes = append(writes, tuples...)
	}
	children, err := r.State.resourcesByParent(ctx, tx, parent.UUID)
	if err != nil {
		return nil, nil, err
	}
	var deletes []Tuple
	for _, child := range children {
		if live[child.Identifier] {
			continue
		}
		tuples, err := r.State.RetireResource(ctx, tx, child)
		if err != nil {
			return nil, nil, err
		}
		deletes = append(deletes, tuples...)
	}
	return writes, deletes, nil
}

func embeddedDescriptorIDs(ctx context.Context, tx *sql.Tx, aasID string) ([]string, error) {
	query, args, err := stateDialect.From(goqu.T("submodel_descriptor").As("s")).Join(goqu.T("aas_descriptor").As("a"), goqu.On(goqu.I("s.aas_descriptor_id").Eq(goqu.I("a.descriptor_id")))).Select(goqu.I("s.id")).Where(goqu.I("a.id").Eq(aasID)).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-EMBEDDED-QUERY: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-EMBEDDED-READ: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("REBAC-EMBEDDED-SCAN: %w", err)
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

func (s *StateStore) resourcesByParent(ctx context.Context, tx *sql.Tx, parent string) ([]StoredResource, error) {
	query, args, err := stateDialect.From("rebac_resource").Select("resource_uuid", "kind", "identifier").Where(goqu.Ex{"scope": s.Scope, "parent_uuid": parent, "deleted_at": nil}).Prepared(true).ToSQL()
	if err != nil {
		return nil, fmt.Errorf("REBAC-EMBEDDED-CHILDQUERY: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("REBAC-EMBEDDED-CHILDREN: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var result []StoredResource
	for rows.Next() {
		var resource StoredResource
		if err := rows.Scan(&resource.UUID, &resource.Kind, &resource.Identifier); err != nil {
			return nil, fmt.Errorf("REBAC-EMBEDDED-CHILDSCAN: %w", err)
		}
		result = append(result, resource)
	}
	return result, rows.Err()
}

// EmbeddedChildren returns current independently protected embeddings for a descriptor.
func (s *StateStore) EmbeddedChildren(ctx context.Context, tx *sql.Tx, parent StoredResource) ([]StoredResource, error) {
	return s.resourcesByParent(ctx, tx, parent.UUID)
}

// EmbeddedDescriptorIDs returns identifiers physically contained in an AAS descriptor.
func EmbeddedDescriptorIDs(ctx context.Context, tx *sql.Tx, aasID string) ([]string, error) {
	return embeddedDescriptorIDs(ctx, tx, aasID)
}
