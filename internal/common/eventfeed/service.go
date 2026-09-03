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
	"strings"
	"time"
)

type Service struct {
	repo   *Repository
	cfg    Config
	build  *Builder
	now    func() time.Time
	logger *slog.Logger
}

func NewService(repo *Repository, cfg Config) *Service {
	return &Service{
		repo:   repo,
		cfg:    cfg,
		build:  NewBuilder(cfg),
		now:    func() time.Time { return time.Now().UTC() },
		logger: slog.Default(),
	}
}

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
	events, hasMore, err := s.findAuthorizedPage(ctx, domain, presentation, query.Limit)
	if err != nil {
		return FeedResponse{}, err
	}
	records, err := toRecords(events, query.Presentation)
	if err != nil {
		return FeedResponse{}, err
	}
	var cursor string
	if hasMore && len(events) > 0 {
		last := events[len(events)-1]
		cursor, err = encodeCursor(last.Seq)
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

func (s *Service) RunRetention(ctx context.Context) (int64, error) {
	locked, err := s.repo.TryRetentionLock(ctx)
	if err != nil {
		return 0, err
	}
	if !locked {
		return 0, nil
	}
	defer s.repo.ReleaseRetentionLock(ctx)

	cutoff := s.now().Add(-(s.cfg.MaxAge + s.cfg.HardDeleteGrace))
	n, err := s.repo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	s.logger.InfoContext(ctx, "event feed retention completed",
		"deleted", n, "cutoff", cutoff.Format(time.RFC3339))
	return n, nil
}

func (s *Service) findAuthorizedPage(ctx context.Context, domain domainQuery, presentation Presentation, limit int) ([]FeedEvent, bool, error) {
	authorizer := currentRecordAuthorizer()
	if authorizer == nil {
		events, err := s.repo.FindPage(ctx, domain, presentation)
		if err != nil {
			return nil, false, err
		}
		hasMore := len(events) > limit
		if hasMore {
			events = events[:limit]
		}
		return events, hasMore, nil
	}
	return s.collectAuthorizedEvents(ctx, domain, presentation, limit, authorizer)
}

func (s *Service) collectAuthorizedEvents(ctx context.Context, domain domainQuery, presentation Presentation, limit int, authorizer RecordAuthorizer) ([]FeedEvent, bool, error) {
	out := make([]FeedEvent, 0, limit)
	hasMore := false
	for round := 0; round < 32; round++ {
		page, err := s.repo.FindPage(ctx, domain, presentation)
		if err != nil {
			return nil, false, err
		}
		rawHasMore := len(page) > limit
		if rawHasMore {
			page = page[:limit]
		}
		if len(page) == 0 {
			break
		}
		for _, event := range page {
			domain.AfterSeq = event.Seq
			if !authorizer.Allow(ctx, event.Type, event.Subject) {
				continue
			}
			if len(out) == limit {
				hasMore = true
				return out, true, nil
			}
			out = append(out, event)
		}
		if !rawHasMore {
			break
		}
	}
	return out, hasMore, nil
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
		return domainQuery{
			AfterSeq: event.Seq,
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
