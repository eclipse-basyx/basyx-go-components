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
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// Service implements the Event Feed API's read, write, and retention logic.
type Service struct {
	repo   *Repository
	cfg    Config
	build  *Builder
	now    func() time.Time
	logger *slog.Logger
}

// NewService creates a Service backed by repo and configured with cfg.
func NewService(repo *Repository, cfg Config) *Service {
	return &Service{
		repo:   repo,
		cfg:    cfg,
		build:  NewBuilder(cfg),
		now:    func() time.Time { return time.Now().UTC() },
		logger: slog.Default(),
	}
}

// Builder returns the event Builder used to construct feed events for this service.
func (s *Service) Builder() *Builder {
	return s.build
}

func (s *Service) Write(ctx context.Context, event FeedEvent) error {
	if s == nil || s.repo == nil {
		return nil
	}
	return s.repo.Save(ctx, event)
}

// WriteTx persists event in the same writer transaction as the model mutation.
func (s *Service) WriteTx(ctx context.Context, tx *sql.Tx, event FeedEvent) error {
	if s == nil || s.repo == nil || tx == nil {
		return nil
	}
	_, err := s.repo.SaveTx(ctx, tx, event)
	return err
}

func (s *Service) Read(ctx context.Context, query FeedQuery) (FeedResponse, error) {
	var err error
	query, err = resolveCursorQuery(query)
	if err != nil {
		return FeedResponse{}, err
	}
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
	presentation := normalizePresentation(query.Presentation)
	query.Presentation = presentation
	events, hasMore, resumeSeq, err := s.findAuthorizedPage(ctx, domain, presentation, query.Limit)
	if err != nil {
		return FeedResponse{}, err
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].Time.Equal(events[j].Time) {
			return events[i].PublishSeq < events[j].PublishSeq
		}
		return events[i].Time.Before(events[j].Time)
	})
	records, err := toRecords(events, query.Presentation)
	if err != nil {
		return FeedResponse{}, err
	}
	var cursor string
	if hasMore {
		cursor, err = encodeQueryCursor(resumeSeq, query)
		if err != nil {
			return FeedResponse{}, err
		}
	}
	updated := s.now()
	if len(events) > 0 {
		updated = events[len(events)-1].Time.UTC()
	}
	return FeedResponse{
		ID:      feedDocumentID(s.now()),
		Updated: updated,
		Records: records,
		Cursor:  cursor,
	}, nil
}

// Capabilities describes the feed's supported event types, filters, and presentation modes for discovery.
func (s *Service) Capabilities() CapabilitiesResponse {
	eventTypes := make(map[string]EventTypeCapabilities, len(allEventTypes()))
	for _, t := range allEventTypes() {
		full, compact := schemaPairForType(t, s.cfg.SchemaBaseURL)
		eventTypes[t] = EventTypeCapabilities{
			SupportsCompact:      true,
			Schemas:              map[string]string{string(PresentationRegular): full, string(PresentationCompact): compact},
			FilterableDataFields: nil,
		}
	}
	return CapabilitiesResponse{
		APIVersion: APIVersion,
		EventTypes: eventTypes,
		Filter: FilterCapabilities{
			FilterableFields:  []string{"event.type", "event.subject", "event.source", "event.dataschema"},
			SupportedPrefixes: []string{"rsql"},
			RSQL:              RSQLCapabilities{Operators: []string{"==", "!=", "=in=", "=out="}},
		},
		Presentation: PresentationCapabilities{
			Supported: []string{string(PresentationRegular), string(PresentationCompact)},
			Default:   string(PresentationRegular),
		},
		MaxAge:      s.cfg.MaxAgePeriod(),
		MaxPageSize: s.cfg.MaxPageSize,
		Auth:        AuthCapabilities{Inherited: true},
	}
}

// RunRetention deletes events past the configured retention window, using an
// advisory lock so only one replica performs the cleanup at a time. It
// returns the number of deleted events.
func (s *Service) RunRetention(ctx context.Context) (int64, error) {
	cutoff := s.now().Add(-(s.cfg.MaxAge + s.cfg.HardDeleteGrace))
	count, err := s.repo.DeleteOlderThan(ctx, cutoff)
	if err == nil && count > 0 {
		s.logger.InfoContext(ctx, "event feed retention completed", "deleted", count, "cutoff", cutoff.Format(time.RFC3339))
	}
	return count, err
}

// RunPublishAssignment assigns durable cursor positions to newly committed rows.
func (s *Service) RunPublishAssignment(ctx context.Context) (int64, error) {
	count, err := s.repo.AssignPublishSeq(ctx, publishBatchSize)
	if err == nil && count > 0 {
		s.logger.DebugContext(ctx, "event feed publish assignment completed", "assigned", count)
	}
	return count, err
}

const authScanRounds = 32

func (s *Service) findAuthorizedPage(ctx context.Context, domain domainQuery, presentation Presentation, limit int) ([]FeedEvent, bool, int64, error) {
	authorizer := currentRecordAuthorizer()
	if authorizer == nil {
		events, err := s.repo.FindPage(ctx, domain, presentation)
		if err != nil {
			return nil, false, 0, err
		}
		hasMore := len(events) > limit
		if hasMore {
			events = events[:limit]
			return events, true, events[len(events)-1].PublishSeq, nil
		}
		return events, false, 0, nil
	}
	return s.collectAuthorizedEvents(ctx, domain, presentation, limit, authorizer)
}

func (s *Service) collectAuthorizedEvents(ctx context.Context, domain domainQuery, presentation Presentation, limit int, authorizer RecordAuthorizer) ([]FeedEvent, bool, int64, error) {
	out := make([]FeedEvent, 0, limit)
	lastScanned := int64(0)
	for round := 0; round < authScanRounds; round++ {
		page, err := s.repo.FindPage(ctx, domain, presentation)
		if err != nil {
			return nil, false, 0, err
		}
		rawHasMore := len(page) > limit
		if rawHasMore {
			page = page[:limit]
		}
		if len(page) == 0 {
			return out, false, 0, nil
		}
		for _, event := range page {
			lastScanned = event.PublishSeq
			domain.AfterSeq = event.PublishSeq
			if !authorizer.AllowEvent(ctx, event) {
				continue
			}
			if len(out) == limit {
				return out, true, out[len(out)-1].PublishSeq, nil
			}
			out = append(out, event)
		}
		if !rawHasMore {
			return out, false, 0, nil
		}
	}
	return out, true, lastScanned, nil
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
	switch normalizePresentation(query.Presentation) {
	case PresentationRegular, PresentationCompact:
	default:
		return newQueryError("EVENTFEED-QUERY-PRESENTATION", "presentation must be REGULAR or COMPACT")
	}
	return nil
}

func normalizePresentation(presentation Presentation) Presentation {
	switch Presentation(strings.ToUpper(strings.TrimSpace(string(presentation)))) {
	case "", PresentationRegular, PresentationFull:
		return PresentationRegular
	case PresentationCompact:
		return PresentationCompact
	default:
		return presentation
	}
}

func (s *Service) buildDomainQuery(ctx context.Context, query FeedQuery, filter *parsedFilter) (domainQuery, error) {
	if strings.TrimSpace(query.Cursor) != "" {
		data, err := decodeCursor(query.Cursor)
		if err != nil {
			return domainQuery{}, newQueryError("EVENTFEED-QUERY-CURSOR", err.Error())
		}
		return domainQuery{
			AfterSeq: data.AfterSeq,
			Since:    query.Since,
			Filter:   filter,
			Limit:    query.Limit,
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
		if event.PublishSeq == 0 {
			return domainQuery{}, newQueryError("EVENTFEED-QUERY-LASTEVENT-PENDING",
				"event "+query.LastEventID+" is not yet available to resume from; retry shortly or use the response's cursor")
		}
		return domainQuery{
			AfterSeq: event.PublishSeq,
			Filter:   filter,
			Limit:    query.Limit,
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
		if normalizePresentation(presentation) == PresentationCompact {
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
