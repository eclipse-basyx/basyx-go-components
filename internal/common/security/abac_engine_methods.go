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
)

type mapMethodAndPatternToRights struct {
	Method  string
	Pattern string
	Rights  []grammar.RightsEnum
}

var mapMethodAndPatternToRightsData = []mapMethodAndPatternToRights{
	// description endpoints
	{"GET", "/description", []grammar.RightsEnum{grammar.RightsEnumREAD}},

	// aas registry
	{"GET", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors/{aasIdentifier}/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},

	{"POST", "/query/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // query endpoint

	// sm registry
	{"GET", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodel-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},

	// sm repository
	{"POST", "/query/submodels", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // query endpoint
	{"GET", "/submodels", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodels", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/submodels/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"PATCH", "/submodels/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/$metadata", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PATCH", "/submodels/{submodelIdentifier}/$value", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"GET", "/submodels/{submodelIdentifier}/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$metadata", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$value", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$reference", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/$path", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
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
	{"PUT", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/attachment", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"POST", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/invoke-async/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-status/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/submodel-elements/{idShortPath}/operation-results/{handleId}/$value", []grammar.RightsEnum{grammar.RightsEnumEXECUTE}},
	{"GET", "/submodels/{submodelIdentifier}/$signed", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"GET", "/submodels/{submodelIdentifier}/$value/$signed", []grammar.RightsEnum{grammar.RightsEnumREAD}},

	// aas discovery
	{"GET", "/lookup/shells", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shellsByAssetLink", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // this is one of the reasons why we need this complex mapping
	{"GET", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"DELETE", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
}

// mapMethodAndPathToRights maps an incoming HTTP method+path to required rights.
// It returns ok=false when no mapping is found so callers can deny by default.
func (m *AccessModel) mapMethodAndPathToRights(in EvalInput) ([]grammar.RightsEnum, bool) {
	for _, mapping := range mapMethodAndPatternToRightsData {
		if mapping.Method == in.Method {
			matchPath := stripBasePath(m.basePath, in.Path)
			pattern := m.apiRouter.Find(m.rctx, in.Method, matchPath)
			if pattern == "" {
				continue
			}
			patternWithBase := joinBasePath(m.basePath, pattern)
			mappingWithBase := joinBasePath(m.basePath, mapping.Pattern)
			if mappingWithBase == patternWithBase {
				return mapping.Rights, true
			}
		}
	}
	return nil, false
}

func rightsContainsAll(hay []grammar.RightsEnum, needles []grammar.RightsEnum) bool {
	// If hay contains ALL → automatically has everything
	for _, r := range hay {
		if r == grammar.RightsEnumALL {
			return true
		}
	}

	// Check each needle individually
	for _, n := range needles {
		found := false
		for _, r := range hay {
			if r == n {
				found = true
				break
			}
		}

		// If one needle is missing → fail
		if !found {
			return false
		}
	}

	return true
}
