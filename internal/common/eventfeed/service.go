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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Service implements feed read/write use cases.
type Service struct {
	repo   *Repository
	cfg    Config
	build  *Builder
	now    func() time.Time
	logger *slog.Logger
}

// NewService creates an Event Feed service.
func NewService(repo *Repository, cfg Config) *Service {
	return &Service{
		repo:   repo,
		cfg:    cfg,
		build:  NewBuilder(cfg),
		now:    func() time.Time { return time.Now().UTC() },
		logger: slog.Default(),
	}
}

// Builder returns the CloudEvents builder used by write hooks.
func (s *Service) Builder() *Builder {
	return s.build
}

// Write persists a feed event. Failures are logged and never returned as fatal
// to callers that use Publish* helpers; Write itself returns the error.
func (s *Service) Write(ctx context.Context, event FeedEvent) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.Save(ctx, event)
}

// PublishBestEffort writes an event and logs failures without failing the caller.
func (s *Service) PublishBestEffort(ctx context.Context, event FeedEvent, err error) {
	if err != nil {
		s.logger.WarnContext(ctx, "event feed build failed", "error.code", "EVENTFEED-PUBLISH-BUILD", "error", err)
		return
	}
	if s == nil || !s.cfg.Enabled {
		return
	}
	if writeErr := s.Write(ctx, event); writeErr != nil {
		s.logger.WarnContext(ctx, "event feed write failed", "error.code", "EVENTFEED-PUBLISH-WRITE", "error", writeErr)
	}
}

// Read returns one page of the event feed.
func (s *Service) Read(ctx context.Context, query FeedQuery) (FeedResponse, error) {
	if err := s.validateQuery(query); err != nil {
		return FeedResponse{}, err
	}
	filter, err := parseFilterParam(query.Filter)
	if err != nil {
		return FeedResponse{}, err
	}
	domain, err := s.buildDomainQuery(ctx, query, filter)
	if err != nil {
		return FeedResponse{}, err
	}
	events, err := s.repo.FindPage(ctx, domain, query.Presentation)
	if err != nil {
		return FeedResponse{}, err
	}
	hasMore := len(events) > query.Limit
	if hasMore {
		events = events[:query.Limit]
	}
	records, err := toRecords(events, query.Presentation)
	if err != nil {
		return FeedResponse{}, err
	}
	var cursor string
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		cursor, err = encodeCursor(last.ID, last.Time)
		if err != nil {
			return FeedResponse{}, err
		}
	}
	updated := s.now()
	if len(events) > 0 {
		updated = events[len(events)-1].Time
	}
	return FeedResponse{
		ID:      feedDocumentID(s.now()),
		Updated: updated,
		Records: records,
		Cursor:  cursor,
	}, nil
}

// Capabilities returns the discovery document.
func (s *Service) Capabilities() CapabilitiesResponse {
	eventTypes := make(map[string]EventTypeCapabilities, len(allEventTypes()))
	for _, t := range allEventTypes() {
		full, compact := schemaPairForType(t, s.cfg.SchemaBaseURL)
		eventTypes[t] = EventTypeCapabilities{
			SupportsCompact:      true,
			Schemas:              map[string]string{"FULL": full, "COMPACT": compact},
			FilterableDataFields: nil,
		}
	}
	return CapabilitiesResponse{
		APIVersion: APIVersion,
		EventTypes: eventTypes,
		Filter: FilterCapabilities{
			FilterableFields:  []string{"event.type", "event.subject", "event.source", "event.dataschema"},
			SupportedPrefixes: []string{"rsql"},
			RSQL:              RSQLCapabilities{Operators: []string{"==", "!=", "in", "out"}},
		},
		Presentation: PresentationCapabilities{
			Supported: []string{string(PresentationFull), string(PresentationCompact)},
			Default:   string(PresentationFull),
		},
		MaxAge:      s.cfg.MaxAgePeriod(),
		MaxPageSize: s.cfg.MaxPageSize,
		Auth: AuthCapabilities{
			Public: s.cfg.PublicAccess,
			Bearer: s.cfg.BearerAuth,
		},
	}
}

// RunRetention deletes events older than maxAge + hardDeleteGrace.
func (s *Service) RunRetention(ctx context.Context) (int64, error) {
	cutoff := s.now().Add(-(s.cfg.MaxAge + s.cfg.HardDeleteGrace))
	n, err := s.repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	s.logger.InfoContext(ctx, "event feed retention completed",
		"deleted", n, "cutoff", cutoff.Format(time.RFC3339))
	return n, nil
}

func (s *Service) validateQuery(query FeedQuery) error {
	if query.Limit < 1 {
		return newQueryError("EVENTFEED-QUERY-LIMIT", "limit must be positive")
	}
	if query.Limit > s.cfg.MaxPageSize {
		return newQueryError("EVENTFEED-QUERY-LIMIT",
			fmt.Sprintf("limit %d exceeds maxPageSize %d", query.Limit, s.cfg.MaxPageSize))
	}
	if query.LastEventID != "" && query.Since != nil {
		return newQueryError("EVENTFEED-QUERY-MUTEX", "lastEventId and since are mutually exclusive")
	}
	if query.Since != nil && query.Since.After(s.now()) {
		return newQueryError("EVENTFEED-QUERY-SINCE", "since must not be in the future")
	}
	switch query.Presentation {
	case PresentationFull, PresentationCompact, "":
	default:
		return newQueryError("EVENTFEED-QUERY-PRESENTATION", "presentation must be FULL or COMPACT")
	}
	return nil
}

func (s *Service) buildDomainQuery(ctx context.Context, query FeedQuery, filter *parsedFilter) (domainQuery, error) {
	presentation := query.Presentation
	if presentation == "" {
		presentation = PresentationFull
	}
	_ = presentation

	if strings.TrimSpace(query.Cursor) != "" {
		data, err := decodeCursor(query.Cursor)
		if err != nil {
			return domainQuery{}, newQueryError("EVENTFEED-QUERY-CURSOR", err.Error())
		}
		t := data.AfterTime.UTC()
		return domainQuery{
			AfterID:   data.AfterID,
			AfterTime: &t,
			Filter:    filter,
			Limit:     query.Limit,
		}, nil
	}
	if query.LastEventID != "" {
		event, found, err := s.repo.FindByID(ctx, query.LastEventID)
		if err != nil {
			return domainQuery{}, err
		}
		if !found {
			return domainQuery{}, newQueryError("EVENTFEED-QUERY-LASTEVENT", "unknown lastEventId: "+query.LastEventID)
		}
		t := event.Time.UTC()
		return domainQuery{
			AfterID:   event.ID,
			AfterTime: &t,
			Filter:    filter,
			Limit:     query.Limit,
		}, nil
	}
	return domainQuery{
		Since:  query.Since,
		Filter: filter,
		Limit:  query.Limit,
	}, nil
}

func toRecords(events []FeedEvent, presentation Presentation) ([]FeedRecord, error) {
	records := make([]FeedRecord, 0, len(events))
	for _, e := range events {
		dataJSON := e.DataFull
		schema := e.DataSchemaFull
		if presentation == PresentationCompact {
			dataJSON = e.DataCompact
			schema = e.DataSchemaCompact
		}
		var data map[string]any
		if dataJSON != "" && dataJSON != "null" {
			if err := json.Unmarshal([]byte(dataJSON), &data); err != nil {
				return nil, fmt.Errorf("EVENTFEED-READ-DATAJSON: %w", err)
			}
		}
		records = append(records, FeedRecord{
			SpecVersion: CloudEventsSpecVersion,
			ID:          e.ID,
			Time:        e.Time.UTC(),
			Subject:     e.Subject,
			Type:        e.Type,
			Source:      e.Source,
			DataSchema:  schema,
			Data:        data,
		})
	}
	return records, nil
}
