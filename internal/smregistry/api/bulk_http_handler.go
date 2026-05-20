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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package smregistryapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

// BulkHTTPHandler registers SSP-003 bulk endpoints for Submodel Registry APIs.
type BulkHTTPHandler struct {
	service *BulkService
}

// NewBulkHTTPHandler creates a new Submodel bulk HTTP handler.
func NewBulkHTTPHandler(service *BulkService) *BulkHTTPHandler {
	return &BulkHTTPHandler{service: service}
}

// RegisterRoutes registers bulk endpoints on the provided router.
func (h *BulkHTTPHandler) RegisterRoutes(router chi.Router, includeAsyncLookupRoutes bool) {
	router.Post("/bulk/submodel-descriptors", h.createBulkSubmodelDescriptors)
	router.Put("/bulk/submodel-descriptors", h.putBulkSubmodelDescriptorsByID)
	router.Delete("/bulk/submodel-descriptors", h.deleteBulkSubmodelDescriptorsByID)

	if includeAsyncLookupRoutes {
		router.Get("/bulk/status/{handleId}", h.getBulkAsyncStatus)
		router.Get("/bulk/result/{handleId}", h.getBulkAsyncResult)
	}
}

func (h *BulkHTTPHandler) createBulkSubmodelDescriptors(w http.ResponseWriter, r *http.Request) {
	var descriptors []model.SubmodelDescriptor
	if !decodeJSONBody(r, &descriptors) {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-CREATE-DECODEBODY invalid request body"),
			http.StatusBadRequest,
			componentName,
			"CreateBulkSubmodelDescriptors",
			"DecodeBody",
		))
		return
	}
	if len(descriptors) == 0 {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-CREATE-EMPTYBODY request body must contain at least one descriptor"),
			http.StatusBadRequest,
			componentName,
			"CreateBulkSubmodelDescriptors",
			"EmptyBody",
		))
		return
	}

	writeResponse(w, h.service.StartCreate(r.Context(), descriptors))
}

func (h *BulkHTTPHandler) putBulkSubmodelDescriptorsByID(w http.ResponseWriter, r *http.Request) {
	var descriptors []model.SubmodelDescriptor
	if !decodeJSONBody(r, &descriptors) {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-PUT-DECODEBODY invalid request body"),
			http.StatusBadRequest,
			componentName,
			"PutBulkSubmodelDescriptorsById",
			"DecodeBody",
		))
		return
	}
	if len(descriptors) == 0 {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-PUT-EMPTYBODY request body must contain at least one descriptor"),
			http.StatusBadRequest,
			componentName,
			"PutBulkSubmodelDescriptorsById",
			"EmptyBody",
		))
		return
	}

	writeResponse(w, h.service.StartPut(r.Context(), descriptors))
}

func (h *BulkHTTPHandler) deleteBulkSubmodelDescriptorsByID(w http.ResponseWriter, r *http.Request) {
	var identifiers []string
	if !decodeJSONBody(r, &identifiers) {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-DELETE-DECODEBODY invalid request body"),
			http.StatusBadRequest,
			componentName,
			"DeleteBulkSubmodelDescriptorsById",
			"DecodeBody",
		))
		return
	}
	if len(identifiers) == 0 {
		writeResponse(w, common.NewErrorResponse(
			common.NewErrBadRequest("SMR-BULK-DELETE-EMPTYBODY request body must contain at least one descriptor identifier"),
			http.StatusBadRequest,
			componentName,
			"DeleteBulkSubmodelDescriptorsById",
			"EmptyBody",
		))
		return
	}

	writeResponse(w, h.service.StartDelete(r.Context(), identifiers))
}

func (h *BulkHTTPHandler) getBulkAsyncStatus(w http.ResponseWriter, r *http.Request) {
	handleID := chi.URLParam(r, "handleId")
	resp := h.service.GetStatus(r.Context(), handleID)

	if resp.Code == http.StatusOK {
		if body, ok := resp.Body.(map[string]any); ok {
			if retryAfter, okRetry := body["retryAfter"].(int); okRetry {
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				delete(body, "retryAfter")
				resp.Body = body
			}
		}
	}

	writeResponse(w, resp)
}

func (h *BulkHTTPHandler) getBulkAsyncResult(w http.ResponseWriter, r *http.Request) {
	handleID := chi.URLParam(r, "handleId")
	writeResponse(w, h.service.GetResult(r.Context(), handleID))
}

func decodeJSONBody(r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	return decoder.Decode(target) == nil
}

func writeResponse(w http.ResponseWriter, response model.ImplResponse) {
	if err := model.EncodeJSONResponse(response.Body, &response.Code, w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
