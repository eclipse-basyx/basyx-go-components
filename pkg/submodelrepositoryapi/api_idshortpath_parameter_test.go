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

package openapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type unreachableSubmodelRepositoryService struct {
	SubmodelRepositoryAPIAPIServicer
}

var routeParameterPattern = regexp.MustCompile(`\{([^}]+)\}`)

// invalidRoutedIDShortPaths pairs a request target with the idShortPath
// parameter the router extracts from it.
var invalidRoutedIDShortPaths = []struct{ target, idShortPath string }{
	{target: "/request/%2541", idShortPath: "%41"},
	{target: "/request/Coll.x%2Fattachment", idShortPath: "Coll.x%2Fattachment"},
	{target: "/request/Coll%20Prop", idShortPath: "Coll Prop"},
	{target: "/request/list%5B%5D", idShortPath: "list[]"},
}

func TestIDShortPathHandlersRejectInvalidIDShortPaths(t *testing.T) {
	controller := NewSubmodelRepositoryAPIAPIController(&unreachableSubmodelRepositoryService{}, "", "")
	checked := 0
	for name, route := range controller.Routes() {
		if !strings.Contains(route.Pattern, "{idShortPath}") {
			continue
		}
		checked++
		for _, invalid := range invalidRoutedIDShortPaths {
			t.Run(name+"/"+invalid.idShortPath, func(t *testing.T) {
				assertHandlerRejectsIDShortPath(t, route, invalid.target, invalid.idShortPath)
			})
		}
	}
	if checked == 0 {
		t.Fatal("no route with an idShortPath parameter was checked")
	}
}

func assertHandlerRejectsIDShortPath(t *testing.T, route Route, target string, idShortPath string) {
	t.Helper()
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("handler reached the service with idShortPath %q: %v", idShortPath, recovered)
		}
	}()
	request := httptest.NewRequest(route.Method, target, strings.NewReader("{}"))
	for _, match := range routeParameterPattern.FindAllStringSubmatch(route.Pattern, -1) {
		value := "x"
		if match[1] == "idShortPath" {
			value = idShortPath
		}
		addRouteParam(request, match[1], value)
	}
	response := httptest.NewRecorder()
	route.HandlerFunc(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("%s %s returned %d, want %d: %s", route.Method, route.Pattern, response.Code, http.StatusBadRequest, response.Body.String())
	}
}

type idShortPathRecordingService struct {
	SubmodelRepositoryAPIAPIServicer
	idShortPath string
}

func (s *idShortPathRecordingService) GetSubmodelElementByPathSubmodelRepo(_ context.Context, _ string, idShortPath string, _ string, _ string) (model.ImplResponse, error) {
	s.idShortPath = idShortPath
	return model.Response(http.StatusOK, nil), nil
}

func TestIDShortPathHandlerPassesTheDecodedPathToTheService(t *testing.T) {
	for _, testCase := range []struct{ target, routed, want string }{
		{target: "/request/List%5b0%5d.Prop", routed: "List%5b0%5d.Prop", want: "List[0].Prop"},
		{target: "/request/Coll%2EProp", routed: "Coll%2EProp", want: "Coll.Prop"},
		{target: "/request/List%5B0%5D", routed: "List[0]", want: "List[0]"},
	} {
		service := &idShortPathRecordingService{}
		controller := NewSubmodelRepositoryAPIAPIController(service, "", "")
		request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
		addRouteParam(request, "submodelIdentifier", "x")
		addRouteParam(request, "idShortPath", testCase.routed)
		response := httptest.NewRecorder()
		controller.GetSubmodelElementByPathSubmodelRepo(response, request)
		if service.idShortPath != testCase.want {
			t.Fatalf("%s reached the service as %q, want %q (status %d)", testCase.target, service.idShortPath, testCase.want, response.Code)
		}
	}
}
