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

package auth

import (
	"context"
	"database/sql"
)

type reBACStateContextKey struct{}

// ReBACState records desired relationship-based authorization state inside
// the transaction of a resource mutation. Identifiers are the public
// identifiers of the resources. Implementations are only present while ReBAC
// is enabled; otherwise every Record* helper is a no-op.
type ReBACState interface {
	ResourceCreated(ctx context.Context, tx *sql.Tx, resource SemanticResourceKind, identifier string) error
	ResourceDeleted(ctx context.Context, tx *sql.Tx, resource SemanticResourceKind, identifier string) error
	SubmodelReferenceRemoved(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error
}

// WithReBACState attaches the ReBAC state recorder to ctx.
func WithReBACState(ctx context.Context, state ReBACState) context.Context {
	if state == nil {
		return ctx
	}
	return context.WithValue(ctx, reBACStateContextKey{}, state)
}

// ReBACStateFromContext returns the attached recorder or nil.
func ReBACStateFromContext(ctx context.Context) ReBACState {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(reBACStateContextKey{}).(ReBACState)
	return state
}

// RecordReBACResourceCreated makes an authenticated creator the owner of a new
// identifiable. Call it after the resource row was inserted in tx.
func RecordReBACResourceCreated(ctx context.Context, tx *sql.Tx, resource SemanticResourceKind, identifier string) error {
	state := ReBACStateFromContext(ctx)
	if state == nil {
		return nil
	}
	return state.ResourceCreated(ctx, tx, resource, identifier)
}

// RecordReBACResourceDeleted removes the grants and links of an identifiable.
// Call it in tx before the resource row is deleted.
func RecordReBACResourceDeleted(ctx context.Context, tx *sql.Tx, resource SemanticResourceKind, identifier string) error {
	state := ReBACStateFromContext(ctx)
	if state == nil {
		return nil
	}
	return state.ResourceDeleted(ctx, tx, resource, identifier)
}

// RecordReBACSubmodelReferenceRemoved removes the approved inheritance link
// between an AAS and a Submodel whose reference was removed in tx.
func RecordReBACSubmodelReferenceRemoved(ctx context.Context, tx *sql.Tx, aasIdentifier string, submodelIdentifier string) error {
	state := ReBACStateFromContext(ctx)
	if state == nil {
		return nil
	}
	return state.SubmodelReferenceRemoved(ctx, tx, aasIdentifier, submodelIdentifier)
}
