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
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DPPRepositoryRouter exposes DPP repository service operations as HTTP handlers.
//
// Fields:
//   - service: DPP repository service used by all HTTP route handlers
type DPPRepositoryRouter struct {
	service *DPPRepositoryService
}

// NewDPPRepositoryRouter creates an HTTP router adapter for the DPP repository service.
//
// Parameters:
//   - service: DPP repository service that executes domain operations
//
// Returns:
//   - *DPPRepositoryRouter: Router adapter exposing the service through HTTP handlers
func NewDPPRepositoryRouter(service *DPPRepositoryService) *DPPRepositoryRouter {
	return &DPPRepositoryRouter{service: service}
}

// OrderedRoutes returns DPP routes in registration order.
//
// Returns:
//   - []Route: DPP API routes ordered for deterministic registration
func (r *DPPRepositoryRouter) OrderedRoutes() []Route {
	return []Route{
		{"ReadDPPById", http.MethodGet, "/v1/dpps/{dppId}", r.ReadDPPById},
		{"DeleteDPPById", http.MethodDelete, "/v1/dpps/{dppId}", r.DeleteDPPById},
		{"UpdateDPPById", http.MethodPatch, "/v1/dpps/{dppId}", r.UpdateDPPById},
		{"CreateDPP", http.MethodPost, "/v1/dpps", r.CreateDPP},
		{"ReadDPPByProductId", http.MethodGet, "/v1/dppsByProductId/{productId}", r.ReadDPPByProductId},
		{"ReadDPPVersionByIdAndDate", http.MethodGet, "/v1/dppsByIdAndDate/{dppId}", r.ReadDPPVersionByIdAndDate},
		{"ReadDPPIdsByProductIds", http.MethodPost, "/v1/dppsByProductIds", r.ReadDPPIdsByProductIds},
		{"ReadDataElement", http.MethodGet, "/v1/dpps/{dppId}/elements/*", r.ReadDataElement},
		{"UpdateDataElement", http.MethodPut, "/v1/dpps/{dppId}/elements/*", r.UpdateDataElement},
	}
}

// Routes returns DPP routes keyed by operation name.
//
// Returns:
//   - Routes: DPP API routes keyed by operation name
func (r *DPPRepositoryRouter) Routes() Routes {
	routes := make(Routes)
	for _, route := range r.OrderedRoutes() {
		routes[route.Name] = route
	}
	return routes
}

// ReadDPPById handles GET /v1/dpps/{dppId}.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing dppId path and representation query values
func (r *DPPRepositoryRouter) ReadDPPById(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPById(req.Context(), pathParam(req, "dppId"), representation)
	r.write(w, response, err)
}

// DeleteDPPById handles DELETE /v1/dpps/{dppId}.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing the dppId path value
func (r *DPPRepositoryRouter) DeleteDPPById(w http.ResponseWriter, req *http.Request) {
	response, err := r.service.DeleteDPPById(req.Context(), pathParam(req, "dppId"))
	r.write(w, response, err)
}

// UpdateDPPById handles PATCH /v1/dpps/{dppId}.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing the dppId path value and patch body
func (r *DPPRepositoryRouter) UpdateDPPById(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("UPDDPP", err), nil)
		return
	}
	response, err := r.service.UpdateDPPFromJSON(req.Context(), pathParam(req, "dppId"), body)
	r.write(w, response, err)
}

// CreateDPP handles POST /v1/dpps.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing the DPP creation body
func (r *DPPRepositoryRouter) CreateDPP(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("CREATEDPP", err), nil)
		return
	}
	response, err := r.service.CreateDPPFromJSON(req.Context(), body)
	r.write(w, response, err)
}

// ReadDPPByProductId handles GET /v1/dppsByProductId/{productId}.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing productId path and representation query values
func (r *DPPRepositoryRouter) ReadDPPByProductId(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPByProductId(req.Context(), pathParam(req, "productId"), representation)
	r.write(w, response, err)
}

// ReadDPPVersionByIdAndDate handles GET /v1/dppsByIdAndDate/{dppId}.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP response
//   - req: HTTP request containing dppId path, date query, and representation query values
func (r *DPPRepositoryRouter) ReadDPPVersionByIdAndDate(w http.ResponseWriter, req *http.Request) {
	date, err := time.Parse(time.RFC3339Nano, req.URL.Query().Get("date"))
	if err != nil {
		r.write(w, errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-READDATE-PARSE parse date query parameter: %w", err)), nil)
		return
	}
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPVersionByIdAndDate(req.Context(), pathParam(req, "dppId"), date, representation)
	r.write(w, response, err)
}

// ReadDPPIdsByProductIds handles POST /v1/dppsByProductIds.
//
// Parameters:
//   - w: HTTP response writer used to encode the DPP ID search response
//   - req: HTTP request containing product IDs body plus limit and cursor query values
func (r *DPPRepositoryRouter) ReadDPPIdsByProductIds(w http.ResponseWriter, req *http.Request) {
	var request ReadDppIdsByProductIdsRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, maxDPPRequestBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		r.write(w, requestBodyDecodeErrorResponse("READIDS", err), nil)
		return
	}
	if err := validateReadDPPIdsRequest(request); err != nil {
		r.write(w, errorResponse(http.StatusBadRequest, err), nil)
		return
	}
	limit, invalid := queryReadDPPIDsLimit(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDPPIdsByProductIds(req.Context(), request, limit, req.URL.Query().Get("cursor"))
	r.write(w, response, err)
}

// ReadDataElement handles GET /v1/dpps/{dppId}/elements/{elementPath}.
//
// Parameters:
//   - w: HTTP response writer used to encode the element response
//   - req: HTTP request containing dppId, elementPath, and representation values
func (r *DPPRepositoryRouter) ReadDataElement(w http.ResponseWriter, req *http.Request) {
	representation, invalid := queryRepresentation(req)
	if invalid != nil {
		r.write(w, *invalid, nil)
		return
	}
	response, err := r.service.ReadDataElement(req.Context(), pathParam(req, "dppId"), elementPathParam(req), representation)
	r.write(w, response, err)
}

// UpdateDataElement handles PUT /v1/dpps/{dppId}/elements/{elementPath}.
//
// Parameters:
//   - w: HTTP response writer used to encode the element response
//   - req: HTTP request containing dppId, elementPath, and replacement element body
func (r *DPPRepositoryRouter) UpdateDataElement(w http.ResponseWriter, req *http.Request) {
	body, err := readRequestBody(w, req)
	if err != nil {
		r.write(w, requestBodyErrorResponse("UPDELEM", err), nil)
		return
	}
	response, err := r.service.UpdateDataElementFromJSON(req.Context(), pathParam(req, "dppId"), elementPathParam(req), body)
	r.write(w, response, err)
}

func (r *DPPRepositoryRouter) write(w http.ResponseWriter, response ImplResponse, err error) {
	if err != nil {
		response = errorResponse(http.StatusInternalServerError, err)
	}
	_ = EncodeJSONResponse(response.Body, &response.Code, w)
}
