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

package auth

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

type aasCapabilities struct {
	CanUpdate bool `json:"canUpdate"`
}

type capabilityError struct {
	Message string `json:"message"`
}

func isAASCapabilitiesPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 4 && parts[0] == "shells" && parts[2] == "$access" && parts[3] == "capabilities"
}

func (repo *resourceBoundRepository) serveAASCapabilities(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	if _, err := boundActor(r.Context()); err != nil {
		writeCapabilityResponse(w, http.StatusUnauthorized, capabilityError{Message: "authentication required"})
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeCapabilityResponse(w, http.StatusMethodNotAllowed, capabilityError{Message: "method not allowed"})
		return
	}
	target, err := parseBoundTarget(r.URL.EscapedPath(), repo.basePath)
	if err != nil || target.Kind != "aas" || target.AAS == "" || !target.Access || target.Suffix != "capabilities" {
		writeCapabilityNotFound(w)
		return
	}
	target.Access = false
	target.Suffix = ""
	repo.evaluateAASCapabilities(w, r, target)
}

func (repo *resourceBoundRepository) evaluateAASCapabilities(w http.ResponseWriter, r *http.Request, target boundTarget) {
	tx, err := repo.db.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		writeCapabilityInternalError(r, w, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	revision, err := repo.readRevision(r.Context(), tx)
	if err != nil {
		writeCapabilityInternalError(r, w, err)
		return
	}
	visible, err := repo.checkAASCapability(r, tx, target, revision, http.MethodGet)
	if err != nil {
		writeCapabilityInternalError(r, w, err)
		return
	}
	if !visible {
		writeCapabilityNotFound(w)
		return
	}
	canUpdate, err := repo.checkAASCapability(r, tx, target, revision, http.MethodPut)
	if err != nil {
		writeCapabilityInternalError(r, w, err)
		return
	}
	writeCapabilityResponse(w, http.StatusOK, aasCapabilities{CanUpdate: canUpdate})
}

func (repo *resourceBoundRepository) checkAASCapability(r *http.Request, tx *sql.Tx, target boundTarget, revision int64, method string) (bool, error) {
	path := strings.SplitN(r.URL.Path, "/$access/capabilities", 2)[0]
	routePath := strings.SplitN(r.URL.EscapedPath(), "/$access/capabilities", 2)[0]
	request := r.Clone(r.Context())
	request.Method = method
	request.URL.Path = path
	request.URL.RawPath = routePath
	_, err := repo.authorizeRequest(request, tx, target, revision)
	if err == nil {
		return true, nil
	}
	if capabilityDenial(err) {
		return false, nil
	}
	return false, err
}

func capabilityDenial(err error) bool {
	if errors.Is(err, sql.ErrNoRows) {
		return true
	}
	var apiErr *boundHTTPError
	return errors.As(err, &apiErr) && (apiErr.status == http.StatusForbidden || apiErr.status == http.StatusNotFound)
}

func writeCapabilityNotFound(w http.ResponseWriter) {
	writeCapabilityResponse(w, http.StatusNotFound, capabilityError{Message: "resource not found"})
}

func writeCapabilityInternalError(r *http.Request, w http.ResponseWriter, err error) {
	slog.ErrorContext(r.Context(), "capability evaluation failed", "error.code", "REBAC-CAPABILITY-EVALUATE", "error", err)
	writeCapabilityResponse(w, http.StatusInternalServerError, capabilityError{Message: "capability check failed"})
}

func writeCapabilityResponse(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
