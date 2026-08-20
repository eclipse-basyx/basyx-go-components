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
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/go-chi/chi/v5"
)

// AASXAsyncFileServerStatusAPIAPIController binds asynchronous status requests.
type AASXAsyncFileServerStatusAPIAPIController struct {
	service      AASXAsyncFileServerStatusAPIAPIServicer
	errorHandler ErrorHandler
	contextPath  string
}

// AASXAsyncFileServerStatusAPIAPIOption configures the asynchronous status controller.
type AASXAsyncFileServerStatusAPIAPIOption func(*AASXAsyncFileServerStatusAPIAPIController)

// WithAASXAsyncFileServerStatusAPIAPIErrorHandler configures controller error handling.
func WithAASXAsyncFileServerStatusAPIAPIErrorHandler(handler ErrorHandler) AASXAsyncFileServerStatusAPIAPIOption {
	return func(controller *AASXAsyncFileServerStatusAPIAPIController) { controller.errorHandler = handler }
}

// NewAASXAsyncFileServerStatusAPIAPIController creates an asynchronous status controller.
func NewAASXAsyncFileServerStatusAPIAPIController(service AASXAsyncFileServerStatusAPIAPIServicer, contextPath string, options ...AASXAsyncFileServerStatusAPIAPIOption) *AASXAsyncFileServerStatusAPIAPIController {
	controller := &AASXAsyncFileServerStatusAPIAPIController{service: service, errorHandler: DefaultErrorHandler, contextPath: normalizeContextPath(contextPath)}
	for _, option := range options {
		option(controller)
	}
	return controller
}

// Routes returns the asynchronous status routes exposed by the controller.
func (controller *AASXAsyncFileServerStatusAPIAPIController) Routes() Routes {
	return Routes{
		"GetAasxAsyncStatus": {
			Method: strings.ToUpper("Get"), Pattern: controller.contextPath + "/packages-async/status/{handleId}", HandlerFunc: controller.GetAasxAsyncStatus,
		},
	}
}

// GetAasxAsyncStatus returns or redirects the status of an asynchronous package upload.
func (controller *AASXAsyncFileServerStatusAPIAPIController) GetAasxAsyncStatus(writer http.ResponseWriter, request *http.Request) {
	handleID := chi.URLParam(request, "handleId")
	if handleID == "" {
		controller.errorHandler(writer, request, &RequiredError{"handleId"}, nil)
		return
	}
	result, err := controller.service.GetAasxAsyncStatus(request.Context(), handleID)
	if err != nil {
		controller.errorHandler(writer, request, err, &result)
		return
	}
	if redirect, ok := result.Body.(Redirect); ok {
		redirect.Location = common.ContextualizeAPIResourceLocation(request, redirect.Location, "/packages-async")
		result.Body = redirect
	}
	_ = EncodeJSONResponse(result.Body, &result.Code, writer)
}
