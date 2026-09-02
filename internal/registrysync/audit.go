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

// Package registrysync provides shared descriptor construction and audit attribution for repository-to-registry synchronization.
package registrysync

import (
	"context"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

const (
	aasRegistrySyncEndpoint      = "internal:aas-registry-sync"
	submodelRegistrySyncEndpoint = "internal:submodel-registry-sync"

	aasRegistrySyncUpsertOperation      = "AASRegistrySync.UpsertDescriptor"
	aasRegistrySyncDeleteOperation      = "AASRegistrySync.DeleteDescriptor"
	submodelRegistrySyncUpsertOperation = "SubmodelRegistrySync.UpsertDescriptor"
	submodelRegistrySyncDeleteOperation = "SubmodelRegistrySync.DeleteDescriptor"
)

// WithAASRegistryAudit attributes an internal AAS descriptor mutation while preserving system audit attribution.
func WithAASRegistryAudit(ctx context.Context, operation string) context.Context {
	return withRegistryAudit(ctx, operation, aasRegistrySyncEndpoint)
}

// WithSubmodelRegistryAudit attributes an internal Submodel descriptor mutation while preserving system audit attribution.
func WithSubmodelRegistryAudit(ctx context.Context, operation string) context.Context {
	return withRegistryAudit(ctx, operation, submodelRegistrySyncEndpoint)
}

// WithAASRegistrySyncUpsertAudit attributes an internal AAS descriptor upsert.
func WithAASRegistrySyncUpsertAudit(ctx context.Context) context.Context {
	return WithAASRegistryAudit(ctx, aasRegistrySyncUpsertOperation)
}

// WithAASRegistrySyncDeleteAudit attributes an internal AAS descriptor deletion.
func WithAASRegistrySyncDeleteAudit(ctx context.Context) context.Context {
	return WithAASRegistryAudit(ctx, aasRegistrySyncDeleteOperation)
}

// WithSubmodelRegistrySyncUpsertAudit attributes an internal Submodel descriptor upsert.
func WithSubmodelRegistrySyncUpsertAudit(ctx context.Context) context.Context {
	return WithSubmodelRegistryAudit(ctx, submodelRegistrySyncUpsertOperation)
}

// WithSubmodelRegistrySyncDeleteAudit attributes an internal Submodel descriptor deletion.
func WithSubmodelRegistrySyncDeleteAudit(ctx context.Context) context.Context {
	return WithSubmodelRegistryAudit(ctx, submodelRegistrySyncDeleteOperation)
}

func withRegistryAudit(ctx context.Context, operation string, endpoint string) context.Context {
	audit := history.FromContext(ctx)
	if audit.AuthorizationResult == history.AuthorizationResultSystemInternal && audit.HTTPMethod == history.AuditHTTPMethodSystem {
		return ctx
	}
	return history.ContextWithAuditOperation(ctx, operation, endpoint)
}
