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

package aasenvironment

import (
	"context"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/history"
)

const (
	aasPreconfigurationActorSubject = "system:aas-preconfiguration"
	aasPreconfigurationActorIssuer  = "basyx:aasenvironmentservice"
	aasPreconfigurationClientID     = "aasenvironmentservice"
	aasPreconfigurationOperation    = "AASPreconfiguration"
	aasPreconfigurationEndpoint     = "startup:aas-preconfiguration"
	aasPreconfigurationIDPrefix     = "aas-preconfiguration"

	aasRegistrySyncEndpoint      = "internal:aas-registry-sync"
	submodelRegistrySyncEndpoint = "internal:submodel-registry-sync"

	aasRegistrySyncUpsertOperation         = "AASRegistrySync.UpsertDescriptor"
	aasRegistrySyncDeleteOperation         = "AASRegistrySync.DeleteDescriptor"
	aasRegistrySyncUpsertEmbeddedOperation = "AASRegistrySync.UpsertEmbeddedSubmodelDescriptor"
	aasRegistrySyncDeleteEmbeddedOperation = "AASRegistrySync.DeleteEmbeddedSubmodelDescriptor"
	submodelRegistrySyncUpsertOperation    = "SubmodelRegistrySync.UpsertDescriptor"
	submodelRegistrySyncDeleteOperation    = "SubmodelRegistrySync.DeleteDescriptor"
)

// ContextWithAASPreconfigurationAudit stores synthetic audit metadata for startup AAS imports.
//
// Use this helper when the AAS Environment Service imports configured AASX,
// JSON, or XML files during startup. Preconfiguration runs outside an HTTP
// request and therefore has no authenticated caller, request headers, or ABAC
// decision to copy. The returned context marks resulting history rows and WORM
// evidence artifacts as trusted internal service work instead of leaving audit
// attribution empty or inventing a human identity.
//
// The synthetic audit context uses the AAS preconfiguration system actor,
// `SYSTEM_INTERNAL` authorization result, `SYSTEM` HTTP method, and a generated
// request/correlation identifier. Registry synchronization that is triggered by
// the same startup import keeps this attribution so descriptor side effects can
// be traced back to the preconfiguration job.
//
// Do not use this helper for normal HTTP mutations. Request handling should keep
// the audit context produced by history.AuditContextMiddleware so end-user OIDC,
// request, and ABAC metadata remain attached to the history row.
//
// Parameters:
//   - ctx: Base context for the startup preconfiguration import.
//
// Returns:
//   - context.Context: Context containing synthetic AAS preconfiguration audit
//     metadata.
func ContextWithAASPreconfigurationAudit(ctx context.Context) context.Context {
	correlationID := history.NewAuditID(aasPreconfigurationIDPrefix)
	return history.ContextWithSystemAudit(ctx, history.SystemAuditOptions{
		ActorSubject:  aasPreconfigurationActorSubject,
		ActorIssuer:   aasPreconfigurationActorIssuer,
		ClientID:      aasPreconfigurationClientID,
		Operation:     aasPreconfigurationOperation,
		Endpoint:      aasPreconfigurationEndpoint,
		CorrelationID: correlationID,
		IDPrefix:      aasPreconfigurationIDPrefix,
	})
}

func aasRegistryAddAuditMetadataIfNotAvailable(ctx context.Context, operation string) context.Context {
	if contextHasAuditMetadata(ctx) {
		return ctx
	}
	return history.ContextWithAuditOperation(ctx, operation, aasRegistrySyncEndpoint)
}

func submodelRegistryAddAuditMetadataIfNotAvailable(ctx context.Context, operation string) context.Context {
	if contextHasAuditMetadata(ctx) {
		return ctx
	}
	return history.ContextWithAuditOperation(ctx, operation, submodelRegistrySyncEndpoint)
}

func contextHasAuditMetadata(ctx context.Context) bool {
	audit := history.FromContext(ctx)
	return audit.ActorSubject == aasPreconfigurationActorSubject &&
		audit.ActorIssuer == aasPreconfigurationActorIssuer &&
		audit.ClientID == aasPreconfigurationClientID &&
		audit.AuthorizationResult == history.AuthorizationResultSystemInternal &&
		audit.Operation == aasPreconfigurationOperation &&
		audit.Endpoint == aasPreconfigurationEndpoint &&
		audit.HTTPMethod == history.AuditHTTPMethodSystem
}
