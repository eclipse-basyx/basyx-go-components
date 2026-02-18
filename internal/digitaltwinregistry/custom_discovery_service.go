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
*******************************************************************************/
// Author: Martin Stemmer ( Fraunhofer IESE )

// Package digitaltwinregistry package implements a custom discovery service for the Digital Twin Registry.
package digitaltwinregistry

import (
	"context"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

// CustomDiscoveryService wraps the default discovery service to allow custom logic.
type CustomDiscoveryService struct {
	*discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService
}

// NewCustomDiscoveryService constructs a custom discovery service wrapper.
func NewCustomDiscoveryService(base *discoveryapiinternal.AssetAdministrationShellBasicDiscoveryAPIAPIService) *CustomDiscoveryService {
	return &CustomDiscoveryService{AssetAdministrationShellBasicDiscoveryAPIAPIService: base}
}

// SearchAllAssetAdministrationShellIdsByAssetLink Custom logic for /lookup/shellsbyAssetLink
func (s *CustomDiscoveryService) SearchAllAssetAdministrationShellIdsByAssetLink(
	ctx context.Context,
	limit int32,
	cursor string,
	assetLink []model.AssetLink,
) (model.ImplResponse, error) {
	createdAfter, _ := CreatedAfterFromContext(ctx)
	if createdAfter != nil {
		query := buildEdcBpnClaimEqualsHeaderExpression(createdAfter)
		ctx = auth.MergeQueryFilter(ctx, query)
	}

	return s.AssetAdministrationShellBasicDiscoveryAPIAPIService.SearchAllAssetAdministrationShellIdsByAssetLink(ctx, limit, cursor, assetLink)
}

// GetAllAssetLinksByID Custom logic for /lookup/shells/{aasIdentifier}
// buildEdcBpnClaimEqualsHeaderExpression creates a logical expression that checks
// whether the Edc-Bpn claim equals the provided header value.
func buildEdcBpnClaimEqualsHeaderExpression(t *time.Time) grammar.Query {
	dt := grammar.DateTimeLiteralPattern(t.UTC())

	timePattern := grammar.ModelStringPattern("$bd#createdAt")
	timeLe := grammar.LogicalExpression{
		Le: grammar.ComparisonItems{
			{DateTimeVal: &dt},
			{Field: &timePattern},
		},
	}

	return grammar.Query{
		Condition: &timeLe,
	}
}
