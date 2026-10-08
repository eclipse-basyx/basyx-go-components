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

package conditional

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClassifyMapsRoutesToTheirTopLevelResource(t *testing.T) {
	cases := []struct {
		pattern string
		mode    Mode
		kind    Kind
		param   string
	}{
		{"/submodels/{submodelIdentifier}", ModeResource, KindSubmodel, "submodelIdentifier"},
		{"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value", ModeResource, KindSubmodel, "submodelIdentifier"},
		{"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", ModeResource, KindSubmodel, "submodelIdentifier"},
		{"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke", ModeVerifyOnly, KindSubmodel, "submodelIdentifier"},
		{"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value", ModeVerifyOnly, KindSubmodel, "submodelIdentifier"},
		{"/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$metadata", ModeResource, KindSubmodel, "submodelIdentifier"},
		{"/shells/{aasIdentifier}/asset-information/thumbnail", ModeResource, KindAAS, "aasIdentifier"},
		{"/shells/{aasIdentifier}/submodel-refs/{submodelIdentifier}", ModeResource, KindAAS, "aasIdentifier"},
		{"/concept-descriptions/{cdIdentifier}", ModeResource, KindConceptDescription, "cdIdentifier"},
		{"/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", ModeResource, KindAASDescriptor, "aasIdentifier"},
		{"/submodel-descriptors/{submodelIdentifier}", ModeResource, KindSubmodelDescriptor, "submodelIdentifier"},
		{"/lookup/shells/{aasIdentifier}", ModeResource, KindDiscoveryEntry, "aasIdentifier"},
		{"/api/v3/packages/{packageId}", ModeResource, KindAASXPackage, "packageId"},
		{"/companies/{companyIdentifier}", ModeResource, KindCompanyDescriptor, "companyIdentifier"},
		{"/v1/dpps/{dppId}/elements/*", ModeResource, KindDPP, "dppId"},
		{"/submodels", ModeCollection, KindSubmodel, ""},
		{"/shells/$reference", ModeCollection, KindAAS, ""},
		{"/v1/dpps", ModeCollection, KindDPP, ""},
		{"/submodels/{submodelIdentifier}/$signed", ModeExcluded, "", ""},
		{"/shells/{aasIdentifier}/$history", ModeExcluded, "", ""},
		{"/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}", ModeExcluded, "", ""},
		{"/submodels/$recent-changes", ModeExcluded, "", ""},
		{"/query/submodels", ModeExcluded, "", ""},
		{"/bulk/shell-descriptors", ModeExcluded, "", ""},
		{"/lookup/shellsByAssetLink", ModeExcluded, "", ""},
		{"/packages-async/status/{handleId}", ModeExcluded, "", ""},
		{"/v1/dppsByIdAndDate/{dppId}", ModeExcluded, "", ""},
		{"/serialization", ModeExcluded, "", ""},
		{"/description", ModeExcluded, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			policy := Classify(tc.pattern)
			require.Equal(t, tc.mode, policy.Mode)
			require.Equal(t, tc.kind, policy.Kind)
			require.Equal(t, tc.param, policy.Param)
		})
	}
}
