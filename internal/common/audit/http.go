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

package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AdminHandler exposes a single producer's audit stream to validated administrators.
// Authorize must return an issuer-scoped actor only after administrator validation.
type AdminHandler struct {
	Repository Repository
	Stream     string
	Authorize  func(*http.Request) (string, error)
	Verify     func(context.Context, string) (ArchiveVerification, error)
}

// ServeHTTP handles bounded search/export pages and complete chain verification.
func (handler AdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "AUDIT-HTTP-METHOD", http.StatusMethodNotAllowed)
		return
	}
	if handler.Authorize == nil {
		http.Error(w, "AUDIT-HTTP-FORBIDDEN", http.StatusForbidden)
		return
	}
	actor, err := handler.Authorize(r)
	if err != nil || actor == "" {
		http.Error(w, "AUDIT-HTTP-FORBIDDEN", http.StatusForbidden)
		return
	}
	filter, err := parseSearchFilter(r, handler.Stream)
	if err != nil {
		http.Error(w, "AUDIT-HTTP-INPUT", http.StatusBadRequest)
		return
	}
	result, err := handler.result(r, filter)
	if err != nil {
		http.Error(w, "AUDIT-HTTP-QUERY", http.StatusServiceUnavailable)
		return
	}
	_, err = handler.Repository.Record(r.Context(), handler.Stream, Event{CorrelationID: uuid.NewString(), Actor: actor, Resource: "/security/rebac/audit", Outcome: "allowed", Payload: Payload{Action: "audit_read"}})
	if err != nil {
		http.Error(w, "AUDIT-HTTP-PERSIST", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(r.URL.Path, "/export") {
		w.Header().Set("Content-Disposition", `attachment; filename="authorization-audit.json"`)
	}
	_ = json.NewEncoder(w).Encode(result)
}

func (handler AdminHandler) result(r *http.Request, filter SearchFilter) (any, error) {
	if strings.HasSuffix(r.URL.Path, "/verify") {
		if handler.Verify != nil {
			return handler.Verify(r.Context(), handler.Stream)
		}
		return handler.Repository.Verify(r.Context(), handler.Stream)
	}
	return handler.Repository.Search(r.Context(), filter)
}

func parseSearchFilter(r *http.Request, stream string) (SearchFilter, error) {
	values := r.URL.Query()
	filter := SearchFilter{Stream: stream, Actor: values.Get("actor"), Resource: values.Get("resource"), Outcome: values.Get("outcome"), CorrelationID: values.Get("correlation"), Limit: 100}
	if raw := values.Get("limit"); raw != "" {
		limit, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || limit == 0 || limit > 1000 {
			return filter, fmt.Errorf("AUDIT-HTTP-LIMIT expected 1 through 1000")
		}
		filter.Limit = uint(limit)
	}
	if raw := values.Get("after"); raw != "" {
		after, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || after < 0 {
			return filter, fmt.Errorf("AUDIT-HTTP-CURSOR invalid sequence")
		}
		filter.AfterSequence = after
	}
	if err := parseSearchTimes(r, &filter); err != nil {
		return filter, err
	}
	return filter, nil
}

func parseSearchTimes(r *http.Request, filter *SearchFilter) error {
	values := r.URL.Query()
	for name, target := range map[string]*time.Time{"from": &filter.From, "until": &filter.Until} {
		if raw := values.Get(name); raw != "" {
			value, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return fmt.Errorf("AUDIT-HTTP-TIME invalid timestamp")
			}
			*target = value
		}
	}
	if !filter.From.IsZero() && !filter.Until.IsZero() && filter.From.After(filter.Until) {
		return fmt.Errorf("AUDIT-HTTP-RANGE invalid time range")
	}

	return nil
}
