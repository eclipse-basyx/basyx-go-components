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

package main

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/testenv"
)

func TestAPIParameterConformance(t *testing.T) {
	testenv.RunAPIParameterConformance(t, submodelRepositoryBaseURL, nil, []testenv.APIConformanceEndpoint{
		{Path: "/query/submodels", Method: "POST", Body: "{}"},
		{Path: "/submodels/$recent-changes"},
		{Path: "/submodels", Filters: []string{"semanticId"}, IdentifierPath: "/submodels/"},
		{Path: "/submodels/$metadata", Filters: []string{"semanticId"}, IdentifierPath: ""},
		{Path: "/submodels/$value", Filters: []string{"semanticId"}, IdentifierPath: ""},
		{Path: "/submodels/$reference", Filters: []string{"semanticId"}, IdentifierPath: ""},
		{Path: "/submodels/$path", Filters: []string{"semanticId"}, IdentifierPath: ""},
	})
}

func TestExactSubmodelReferenceConformance(t *testing.T) {
	testenv.RunSubmodelReferenceConformance(t, submodelRepositoryBaseURL)
}
