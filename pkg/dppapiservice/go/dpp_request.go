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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func readRequestBody(w http.ResponseWriter, req *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, req.Body, maxDPPRequestBodyBytes))
}

func requestBodyErrorResponse(operation string, err error) ImplResponse {
	if isRequestBodyTooLarge(err) {
		return errorResponse(http.StatusRequestEntityTooLarge, fmt.Errorf("DPP-%s-BODYTOOLARGE request body exceeds %d bytes", operation, maxDPPRequestBodyBytes))
	}
	return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-%s-READBODY read request body: %w", operation, err))
}

func requestBodyDecodeErrorResponse(operation string, err error) ImplResponse {
	if isRequestBodyTooLarge(err) {
		return errorResponse(http.StatusRequestEntityTooLarge, fmt.Errorf("DPP-%s-BODYTOOLARGE request body exceeds %d bytes", operation, maxDPPRequestBodyBytes))
	}
	return errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-%s-DECODE decode request body: %w", operation, err))
}

func isRequestBodyTooLarge(err error) bool {
	return strings.Contains(err.Error(), "request body too large")
}

func pathParam(req *http.Request, name string) string {
	return decodePathParam(chi.URLParam(req, name))
}

func elementPathParam(req *http.Request) string {
	return strings.TrimPrefix(decodePathParam(chi.URLParam(req, "*")), "/")
}

func decodePathParam(value string) string {
	decoded := value
	for range 3 {
		next, err := url.PathUnescape(decoded)
		if err != nil || next == decoded {
			return decoded
		}
		decoded = next
	}
	return decoded
}

func queryRepresentation(req *http.Request) (Representation, *ImplResponse) {
	representation := Representation(req.URL.Query().Get("representation"))
	if representation == "" {
		return REPRESENTATION_COMPRESSED, nil
	}
	if !representation.IsValid() {
		response := errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-REPRESENTATION-INVALID invalid representation %q", representation))
		return "", &response
	}
	return representation, nil
}

func queryReadDPPIDsLimit(req *http.Request) (int32, *ImplResponse) {
	rawLimit := req.URL.Query().Get("limit")
	if rawLimit == "" {
		return defaultDPPPageLimit, nil
	}
	parsed, err := strconv.ParseInt(rawLimit, 10, 32)
	if err != nil || parsed < 1 {
		response := errorResponse(http.StatusBadRequest, fmt.Errorf("DPP-READIDS-LIMIT invalid limit"))
		return 0, &response
	}
	return int32(parsed), nil
}

func validateReadDPPIdsRequest(request ReadDppIdsByProductIdsRequest) error {
	if len(request.ProductIds) == 0 {
		return fmt.Errorf("DPP-READIDS-MISSING productIds must contain at least one product id")
	}
	if len(request.ProductIds) > maxDPPProductIDSearchItems {
		return fmt.Errorf("DPP-READIDS-MAXITEMS productIds must contain at most %d product ids", maxDPPProductIDSearchItems)
	}
	for _, productID := range request.ProductIds {
		if strings.TrimSpace(productID) == "" {
			return fmt.Errorf("DPP-READIDS-INVALID productIds must contain only non-empty strings")
		}
	}
	return nil
}
