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

package eventfeed

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/go-chi/chi/v5"
)

const httpComponent = "EVENTFEED"

func RegisterRoutes(r chi.Router, svc *Service) {
	if r == nil || svc == nil || !svc.cfg.Enabled {
		return
	}
	r.Get("/events", svc.handleGetEvents)
}

func (s *Service) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	presentation := Presentation(strings.ToUpper(strings.TrimSpace(q.Get("presentation"))))
	if presentation == "" {
		presentation = PresentationFull
	}
	limit := 100
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "GetEvents", "Limit", errors.New("limit must be a positive integer"))
			return
		}
		limit = n
	}
	var sincePtr *time.Time
	if raw := strings.TrimSpace(q.Get("since")); raw != "" {
		since, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			since, err = time.Parse(time.RFC3339, raw)
		}
		if err != nil {
			writeHTTPError(w, http.StatusBadRequest, "GetEvents", "Since", errors.New("since must be an ISO-8601 timestamp"))
			return
		}
		since = since.UTC()
		sincePtr = &since
	}

	query := FeedQuery{
		LastEventID:  strings.TrimSpace(q.Get("lastEventId")),
		Since:        sincePtr,
		Cursor:       strings.TrimSpace(q.Get("cursor")),
		Filter:       strings.TrimSpace(q.Get("filter")),
		Presentation: presentation,
		Limit:        limit,
	}
	result, err := s.Read(r.Context(), query)
	if err != nil {
		if IsQueryError(err) {
			writeHTTPError(w, http.StatusBadRequest, "GetEvents", "Query", err)
			return
		}
		writeHTTPError(w, http.StatusInternalServerError, "GetEvents", "Read", err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeHTTPError(w http.ResponseWriter, status int, function, info string, err error) {
	_ = model.WriteErrorResponse(w, err, status, httpComponent, function, info)
}
