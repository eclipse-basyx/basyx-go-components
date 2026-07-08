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

package digitaltwinregistry

import (
	"context"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	discoveryapiinternal "github.com/eclipse-basyx/basyx-go-components/internal/discoveryservice/api"
)

func TestGlobalAssetIDDescriptorVisibilityRequiredOnlyWithABAC(t *testing.T) {
	t.Parallel()

	globalAssetIDs := []string{"global-asset"}

	ctx := common.ContextWithConfig(context.Background(), &common.Config{})
	if globalAssetIDDescriptorVisibilityRequired(ctx, globalAssetIDs, false) {
		t.Fatalf("expected no extra globalAssetId visibility filter when ABAC is disabled")
	}

	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	ctx = common.ContextWithConfig(context.Background(), cfg)
	if !globalAssetIDDescriptorVisibilityRequired(ctx, globalAssetIDs, false) {
		t.Fatalf("expected extra globalAssetId visibility filter when ABAC is enabled")
	}
}

func TestGlobalAssetIDDescriptorVisibilityRequiredSkipsEmptyAndUnrestricted(t *testing.T) {
	t.Parallel()

	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	ctx := common.ContextWithConfig(context.Background(), cfg)

	if globalAssetIDDescriptorVisibilityRequired(ctx, nil, false) {
		t.Fatalf("expected no extra visibility filter without globalAssetId values")
	}
	if globalAssetIDDescriptorVisibilityRequired(ctx, []string{"global-asset"}, true) {
		t.Fatalf("expected no extra visibility filter when READ is unrestricted")
	}
}

func TestMergeAssetLinkLookupFilterKeepsDiscoveryMatchingForGlobalAssetIDs(t *testing.T) {
	t.Parallel()

	ctx := restrictedReadContext()
	links := []model.AssetLink{
		{Name: common.GlobalAssetIDAssetLinkName, Value: "global-asset"},
		{Name: "customerPartId", Value: "customer-part"},
	}

	lookupCtx, err := mergeAssetLinkLookupFilter(ctx, links)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if discoveryapiinternal.AssetLinksAlreadyConstrainedFromContext(lookupCtx) {
		t.Fatalf("expected discovery to keep matching asset links when globalAssetId is present")
	}
}

func TestMergeAssetLinkLookupFilterConstrainsSpecificOnlyAssetLinks(t *testing.T) {
	t.Parallel()

	ctx := restrictedReadContext()
	links := []model.AssetLink{{Name: "customerPartId", Value: "customer-part"}}

	lookupCtx, err := mergeAssetLinkLookupFilter(ctx, links)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !discoveryapiinternal.AssetLinksAlreadyConstrainedFromContext(lookupCtx) {
		t.Fatalf("expected specific-only asset links to be marked as constrained")
	}
}

func restrictedReadContext() context.Context {
	cfg := &common.Config{}
	cfg.ABAC.Enabled = true
	ctx := common.ContextWithConfig(context.Background(), cfg)
	read := false
	return auth.WithQueryFilter(ctx, &auth.QueryFilter{
		FormulasByRight: map[grammar.RightsEnum]grammar.LogicalExpression{
			grammar.RightsEnumREAD: {Boolean: &read},
		},
	})
}
