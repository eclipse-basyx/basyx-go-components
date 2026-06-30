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
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	api "github.com/go-chi/chi/v5"
)

type mapMethodAndPatternToRights struct {
	Method  string
	Pattern string
	Rights  []grammar.RightsEnum
}

var mapMethodAndPatternToRightsData = []mapMethodAndPatternToRights{
	// description endpoints
	{"GET", "/description", []grammar.RightsEnum{grammar.RightsEnumREAD}},

	// ABAC policy management
	{"GET", "/security/abac/active-policy", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/security/abac/active-policy/rules", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/security/abac/policy-versions", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/security/abac/policy-versions", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/security/abac/policy-versions/{versionID}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/security/abac/policy-versions/{versionID}/clone", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"POST", "/security/abac/policy-versions/{versionID}/validate", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"POST", "/security/abac/policy-versions/{versionID}/activate", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"POST", "/security/abac/policy-versions/{versionID}/reject", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/security/abac/policy-versions/{versionID}/rules", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/security/abac/policy-versions/{versionID}/rules", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"PATCH", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"POST", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}/duplicate", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"POST", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}/move", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"PUT", "/security/abac/policy-versions/{versionID}/rules/{ruleIndex}/enabled", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/security/abac/policy-versions/{versionID}/definitions", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/security/abac/policy-versions/{versionID}/definitions/{kind}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/security/abac/policy-versions/{versionID}/definitions/{kind}", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/security/abac/policy-versions/{versionID}/definitions/{kind}/{name}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/security/abac/policy-versions/{versionID}/definitions/{kind}/{name}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"PATCH", "/security/abac/policy-versions/{versionID}/definitions/{kind}/{name}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/security/abac/policy-versions/{versionID}/definitions/{kind}/{name}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},

	// aas registry
	{"GET", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"POST", "/bulk/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"PUT", "/bulk/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/bulk/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/bulk/status/{handleId}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/bulk/result/{handleId}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors/{aasIdentifier}/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},

	{"POST", "/query/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // query endpoint

	// sm registry
	{"POST", "/query/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // query endpoint
	{"GET", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"POST", "/bulk/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"PUT", "/bulk/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/bulk/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumDELETE}},

	// concept description repository
	{"POST", "/query/concept-descriptions", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/concept-descriptions", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/concept-descriptions", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/concept-descriptions/$recent-changes", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/concept-descriptions/{cdIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/concept-descriptions/{cdIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/concept-descriptions/{cdIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},

	// sm repository
	{"POST", "/query/submodels", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // query endpoint
	{"GET", "/submodels", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodels", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/submodels/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$recent-changes", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/$history", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"DELETE", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"PATCH", "/submodels/{submodelIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodels/{submodelIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/submodels/{submodelIdentifier}/$value/$signed", []grammar.RightsEnum{grammar.RightsEnumREAD}},

	// aas repository
	{"POST", "/query/shells", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shells", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/shells/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/$recent-changes", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shells/{aasIdentifier}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/$history", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/asset-information", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}/asset-information", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/asset-information/thumbnail", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}/asset-information/thumbnail", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shells/{aasIdentifier}/asset-information/thumbnail", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shells/{aasIdentifier}/submodel-refs", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shells/{aasIdentifier}/submodel-refs", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"DELETE", "/shells/{aasIdentifier}/submodel-refs/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"DELETE", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$metadata", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$value", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/shells/{aasIdentifier}/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},

	// aas environment
	{"POST", "/upload", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"GET", "/serialization", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	// extend mapping for endpoints that are not yet implemented.

	// dpp api
	{"GET", "/v1/dpps/{dppId}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"DELETE", "/v1/dpps/{dppId}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/v1/dpps/{dppId}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"POST", "/v1/dpps", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/v1/dppsByProductId/{productId}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/v1/dppsByIdAndDate/{dppId}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/v1/dppsByProductIds", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/v1/dpps/{dppId}/elements/*", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/v1/dpps/{dppId}/elements/*", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},

	// aas discovery
	{"GET", "/lookup/shells", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shellsByAssetLink", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // this is one of the reasons why we need this complex mapping
	{"GET", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE, grammar.RightsEnumUPDATE}},
	{"DELETE", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
}

// mapMethodAndPathToRights maps an incoming HTTP method+path to required rights.
// It returns:
//   - mapped=false, routeFound=false when the route does not exist
//   - mapped=false, routeFound=true when the route exists but has no rights mapping
//   - mapped=true, routeFound=true with one or more rights alternatives
func (m *AccessModel) mapMethodAndPathToRights(in EvalInput) ([][]grammar.RightsEnum, bool, bool) {
	matchPath := stripBasePath(m.basePath, in.Path)
	rctx := api.NewRouteContext()
	pattern := m.apiRouter.Find(rctx, in.Method, matchPath)
	if pattern == "" {
		return nil, false, false
	}

	patternWithBase := joinBasePath(m.basePath, pattern)
	var alternatives [][]grammar.RightsEnum
	for _, mapping := range mapMethodAndPatternToRightsData {
		if mapping.Method != in.Method {
			continue
		}

		mappingWithBase := joinBasePath(m.basePath, mapping.Pattern)
		if mappingWithBase == patternWithBase {
			alternatives = append(alternatives, mapping.Rights)
		}
	}

	if len(alternatives) > 0 {
		return alternatives, true, true
	}

	return nil, false, true
}
