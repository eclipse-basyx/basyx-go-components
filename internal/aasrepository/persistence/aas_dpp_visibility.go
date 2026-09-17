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

package persistence

import (
	"context"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

const dppIDElementPath = "digitalProductPassportId"

var dppIDVisibilityFragments = []grammar.FragmentStringPattern{
	"$sme#idShort",
	"$sme." + dppIDElementPath + "#idShort",
	"$sme#value",
	"$sme." + dppIDElementPath + "#value",
}

func (s *AssetAdministrationShellDatabase) addDPPIDLookupAuthorization(
	ctx context.Context,
	dataset *goqu.SelectDataset,
	operation string,
) (*goqu.SelectDataset, error) {
	dataset, err := s.addAASAuthorizationFormula(ctx, dataset, operation)
	if err != nil {
		return nil, err
	}

	filterContext := dppIDVisibilityFilterContext(ctx)
	collector, err := grammar.NewResolvedFieldPathCollectorForSMERow("metadata_element")
	if err != nil {
		return nil, common.NewInternalServerError(operation + "-BADSMECOLLECTOR " + err.Error())
	}
	dataset, err = auth.AddFilterQueriesFromContext(filterContext, dataset, dppIDVisibilityFragments, collector)
	if err != nil {
		return nil, common.NewInternalServerError(operation + "-ABACDPPIDVISIBILITY " + err.Error())
	}
	return dataset, nil
}

func dppIDVisibilityFilterContext(ctx context.Context) context.Context {
	queryFilter := auth.GetQueryFilter(ctx)
	if queryFilter == nil || len(queryFilter.Filters) == 0 {
		return ctx
	}

	filters := make(auth.FragmentFilters, len(queryFilter.Filters))
	for fragment, predicate := range queryFilter.Filters {
		normalized := fragment
		if isStructuralSMEFragment(fragment) {
			normalized = grammar.FragmentStringPattern(string(fragment) + "#idShort")
		}
		if existing, found := filters[normalized]; found {
			filters[normalized] = auth.AndFragmentFilterPredicates(existing, predicate)
			continue
		}
		filters[normalized] = predicate
	}
	return auth.WithQueryFilter(ctx, &auth.QueryFilter{Filters: filters})
}

func isStructuralSMEFragment(fragment grammar.FragmentStringPattern) bool {
	value := string(fragment)
	return !strings.Contains(value, "#") &&
		(value == "$sme" || strings.HasPrefix(value, "$sme.") || strings.HasPrefix(value, "$sme["))
}
