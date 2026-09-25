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

package rebac

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const maxResponseBytes = 1 << 20
const maxStreamedObjects = 100000

// Client calls the OpenFGA HTTP API for one pinned store and model.
type Client struct {
	baseURL *url.URL
	config  Config
	http    *http.Client
	metrics clientMetrics
}

// NewClient constructs a bounded OpenFGA client.
func NewClient(config Config) (*Client, error) {
	baseURL, err := validateConfig(config)
	if err != nil {
		return nil, err
	}
	transport := telemetry.HTTPClientTransport(http.DefaultTransport)
	metrics, err := newClientMetrics()
	if err != nil {
		return nil, err
	}
	return &Client{
		baseURL: baseURL,
		config:  config,
		metrics: metrics,
		http: &http.Client{
			Timeout:       config.Timeout,
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func validateConfig(config Config) (*url.URL, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-URL OpenFGA URL must not be blank")
	}
	baseURL, err := url.Parse(config.URL)
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-URL OpenFGA URL must be an absolute HTTP URL")
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-URL OpenFGA URL must use HTTP or HTTPS")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-URL OpenFGA URL must not contain credentials, query, or fragment")
	}
	if strings.TrimSpace(config.StoreID) == "" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-STORE OpenFGA store ID must not be blank")
	}
	if strings.TrimSpace(config.ModelID) == "" {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-MODEL OpenFGA model ID must not be blank")
	}
	if config.Timeout <= 0 {
		return nil, fmt.Errorf("REBAC-CLIENT-VALIDATE-TIMEOUT OpenFGA timeout must be positive")
	}
	return baseURL, nil
}

// Check returns the explicit OpenFGA decision for a tuple. Invalid or malformed
// responses are errors so callers can fail closed.
func (client *Client) Check(ctx context.Context, user, relation, object string, contextualTuples []Tuple) (allowed bool, err error) {
	measure := client.metrics.measure(ctx, "check", -1)
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.check")
	defer span.End()
	if err := validateTuple(Tuple{User: user, Relation: relation, Object: object}); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid tuple")
		return false, err
	}
	if err := validateTuples(contextualTuples); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid contextual tuples")
		return false, err
	}
	payload := checkRequest{AuthorizationModelID: client.config.ModelID, Consistency: higherConsistency, TupleKey: Tuple{User: user, Relation: relation, Object: object}, ContextualTuples: tupleKeys{TupleKeys: contextualTuples}}
	var response checkResponse
	if err := client.doJSON(ctx, http.MethodPost, client.storePath("check"), payload, &response); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "OpenFGA check failed")
		return false, err
	}
	if response.Allowed == nil {
		err := fmt.Errorf("REBAC-CLIENT-CHECK-RESPONSE OpenFGA check response omitted allowed")
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed response")
		return false, err
	}
	return *response.Allowed, nil
}

// BatchCheck returns one result per input request, correlated by CorrelationID.
// A missing, duplicate, or unknown result is an error to prevent partial allows.
func (client *Client) BatchCheck(ctx context.Context, checks []BatchCheckRequest) (results []BatchCheckResult, err error) {
	measure := client.metrics.measure(ctx, "batch_check", len(checks))
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.batch_check")
	defer span.End()
	request, err := buildBatchCheckRequest(client.config.ModelID, checks)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "invalid batch")
		return nil, err
	}
	var response batchCheckResponse
	if err := client.doJSON(ctx, http.MethodPost, client.storePath("batch-check"), request, &response); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "OpenFGA batch check failed")
		return nil, err
	}
	results, err = correlateBatchResults(checks, response.Result)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "malformed response")
		return nil, err
	}
	return results, nil
}

// StreamedListObjects returns every object of one type readable by the actor.
// The streaming endpoint avoids the server-side result cap of ListObjects.
func (client *Client) StreamedListObjects(ctx context.Context, user, relation, objectType string, contextualTuples []Tuple) (objects []string, err error) {
	measure := client.metrics.measure(ctx, "streamed_list_objects", -1)
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.streamed_list_objects")
	defer span.End()
	if err = validateTuple(Tuple{User: user, Relation: relation, Object: objectType + ":candidate"}); err != nil {
		return nil, recordSpanError(span, err)
	}
	if err = validateTuples(contextualTuples); err != nil {
		return nil, recordSpanError(span, err)
	}
	payload := listObjectsRequest{AuthorizationModelID: client.config.ModelID, Consistency: higherConsistency, User: user, Relation: relation, Type: objectType, ContextualTuples: tupleKeys{TupleKeys: contextualTuples}}
	response, err := client.doStream(ctx, client.storePath("streamed-list-objects"), payload)
	if err != nil {
		return nil, recordSpanError(span, err)
	}
	defer func() { _ = response.Body.Close() }()
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 4096), maxResponseBytes)
	prefix := objectType + ":"
	for scanner.Scan() {
		var item streamedListObjectsResponse
		if decodeErr := json.Unmarshal(scanner.Bytes(), &item); decodeErr != nil || !strings.HasPrefix(item.Result.Object, prefix) || len(item.Result.Object) == len(prefix) {
			return nil, recordSpanError(span, fmt.Errorf("REBAC-CLIENT-LISTOBJECTS-RESPONSE malformed streamed object"))
		}
		objects = append(objects, strings.TrimPrefix(item.Result.Object, prefix))
		if len(objects) > maxStreamedObjects {
			return nil, recordSpanError(span, fmt.Errorf("REBAC-CLIENT-LISTOBJECTS-LIMIT streamed object limit exceeded"))
		}
	}
	if err = scanner.Err(); err != nil {
		return nil, recordSpanError(span, fmt.Errorf("REBAC-CLIENT-LISTOBJECTS-READ: %w", err))
	}
	return objects, nil
}

func buildBatchCheckRequest(modelID string, checks []BatchCheckRequest) (batchCheckRequest, error) {
	if len(checks) == 0 {
		return batchCheckRequest{}, fmt.Errorf("REBAC-CLIENT-BATCH-INPUT batch check must contain at least one check")
	}
	request := batchCheckRequest{AuthorizationModelID: modelID, Consistency: higherConsistency, Checks: make([]batchCheckItem, 0, len(checks))}
	seen := make(map[string]struct{}, len(checks))
	for _, check := range checks {
		if strings.TrimSpace(check.CorrelationID) == "" {
			return batchCheckRequest{}, fmt.Errorf("REBAC-CLIENT-BATCH-CORRELATION batch check correlation ID must not be blank")
		}
		if _, ok := seen[check.CorrelationID]; ok {
			return batchCheckRequest{}, fmt.Errorf("REBAC-CLIENT-BATCH-CORRELATION batch check correlation IDs must be unique")
		}
		if err := validateTuple(check.Tuple); err != nil {
			return batchCheckRequest{}, err
		}
		if err := validateTuples(check.ContextualTuples); err != nil {
			return batchCheckRequest{}, err
		}
		seen[check.CorrelationID] = struct{}{}
		request.Checks = append(request.Checks, batchCheckItem{CorrelationID: check.CorrelationID, TupleKey: check.Tuple, ContextualTuples: tupleKeys{TupleKeys: check.ContextualTuples}})
	}
	return request, nil
}

func correlateBatchResults(checks []BatchCheckRequest, received map[string]batchCheckItemResponse) ([]BatchCheckResult, error) {
	if len(received) != len(checks) {
		return nil, fmt.Errorf("REBAC-CLIENT-BATCH-RESPONSE OpenFGA batch check result count does not match request")
	}
	results := make([]BatchCheckResult, 0, len(checks))
	for _, check := range checks {
		result, exists := received[check.CorrelationID]
		if !exists || result.Allowed == nil {
			return nil, fmt.Errorf("REBAC-CLIENT-BATCH-RESPONSE OpenFGA omitted a batch result")
		}
		if len(result.Error) != 0 && string(result.Error) != "null" {
			return nil, fmt.Errorf("REBAC-CLIENT-BATCH-ITEM OpenFGA failed an individual check")
		}
		results = append(results, BatchCheckResult{CorrelationID: check.CorrelationID, Allowed: *result.Allowed})
	}
	return results, nil
}

// Write applies the requested tuple additions and removals. Repeating an
// already-applied desired write has the same final relationship state.
func (client *Client) Write(ctx context.Context, writes, deletes []Tuple) (err error) {
	measure := client.metrics.measure(ctx, "write", -1)
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.write")
	defer span.End()
	if err := validateDesiredChanges(writes, deletes); err != nil {
		return recordSpanError(span, err)
	}
	writes, deletes, err = client.pendingChanges(ctx, writes, deletes)
	if err != nil {
		return recordSpanError(span, err)
	}
	if len(writes) == 0 && len(deletes) == 0 {
		return nil
	}
	payload := writeRequest{AuthorizationModelID: client.config.ModelID}
	if len(writes) > 0 {
		payload.Writes = &tupleKeys{TupleKeys: writes}
	}
	if len(deletes) > 0 {
		payload.Deletes = &tupleKeys{TupleKeys: deletes}
	}
	if err := client.doJSON(ctx, http.MethodPost, client.storePath("write"), payload, nil); err != nil {
		return recordSpanError(span, err)
	}
	return nil
}

func (client *Client) pendingChanges(ctx context.Context, writes, deletes []Tuple) ([]Tuple, []Tuple, error) {
	pendingWrites := make([]Tuple, 0, len(writes))
	for _, tuple := range writes {
		exists, err := client.tupleExists(ctx, tuple)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			pendingWrites = append(pendingWrites, tuple)
		}
	}
	pendingDeletes := make([]Tuple, 0, len(deletes))
	for _, tuple := range deletes {
		exists, err := client.tupleExists(ctx, tuple)
		if err != nil {
			return nil, nil, err
		}
		if exists {
			pendingDeletes = append(pendingDeletes, tuple)
		}
	}
	return pendingWrites, pendingDeletes, nil
}

func (client *Client) tupleExists(ctx context.Context, tuple Tuple) (bool, error) {
	var response readTuplesResponse
	payload := readTuplesRequest{TupleKey: tuple}
	if err := client.doJSON(ctx, http.MethodPost, client.storePath("read"), payload, &response); err != nil {
		return false, err
	}
	for _, current := range response.Tuples {
		if current.Key == tuple {
			return true, nil
		}
	}
	return false, nil
}

func recordSpanError(span trace.Span, err error) error {
	span.RecordError(err)
	span.SetStatus(codes.Error, "OpenFGA request failed")
	return err
}

// ReadModel validates that the configured authorization model still exists and
// is exactly the pinned model ID.
func (client *Client) ReadModel(ctx context.Context) (err error) {
	measure := client.metrics.measure(ctx, "read_model", -1)
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.read_model")
	defer span.End()
	var response readModelResponse
	path := client.storePath("authorization-models", client.config.ModelID)
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return recordSpanError(span, err)
	}
	var model struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.AuthorizationModel, &model); err != nil {
		return recordSpanError(span, fmt.Errorf("REBAC-CLIENT-MODEL-DECODE: %w", err))
	}
	if model.ID != client.config.ModelID {
		return recordSpanError(span, fmt.Errorf("REBAC-CLIENT-MODEL-RESPONSE OpenFGA returned an unexpected authorization model"))
	}
	if err := ValidateAuthorizationModel(response.AuthorizationModel); err != nil {
		return recordSpanError(span, fmt.Errorf("REBAC-CLIENT-MODEL-VALIDATE: %w", err))
	}
	return nil
}

// ReadChanges reads a cursor-addressable page of tuple changes.
func (client *Client) ReadChanges(ctx context.Context, continuationToken string, pageSize int) (page ChangesPage, err error) {
	measure := client.metrics.measure(ctx, "read_changes", -1)
	defer func() { measure(err) }()
	if pageSize < 1 {
		return ChangesPage{}, fmt.Errorf("REBAC-CLIENT-CHANGES-PAGESIZE page size must be positive")
	}
	path := client.storePath("changes")
	query := path.Query()
	query.Set("page_size", fmt.Sprint(pageSize))
	if continuationToken != "" {
		query.Set("continuation_token", continuationToken)
	}
	path.RawQuery = query.Encode()
	var response changesResponse
	if err := client.doJSON(ctx, http.MethodGet, path, nil, &response); err != nil {
		return ChangesPage{}, err
	}
	changes := make([]Change, 0, len(response.Changes))
	for _, change := range response.Changes {
		if err := validateTuple(change.TupleKey); err != nil {
			return ChangesPage{}, fmt.Errorf("REBAC-CLIENT-CHANGES-RESPONSE OpenFGA returned an invalid tuple change")
		}
		changes = append(changes, Change{Tuple: change.TupleKey, Operation: change.Operation, Timestamp: change.Timestamp})
	}
	return ChangesPage{Changes: changes, ContinuationToken: response.ContinuationToken}, nil
}

func validateTuples(tuples []Tuple) error {
	for _, tuple := range tuples {
		if err := validateTuple(tuple); err != nil {
			return err
		}
	}
	return nil
}

func validateDesiredChanges(writes, deletes []Tuple) error {
	if len(writes)+len(deletes) == 0 {
		return fmt.Errorf("REBAC-CLIENT-WRITE-INPUT write must contain at least one tuple change")
	}
	seen := make(map[Tuple]struct{}, len(writes)+len(deletes))
	for _, tuples := range [][]Tuple{writes, deletes} {
		for _, tuple := range tuples {
			if err := validateTuple(tuple); err != nil {
				return err
			}
			if _, exists := seen[tuple]; exists {
				return fmt.Errorf("REBAC-CLIENT-WRITE-DUPLICATE tuple changes must be unique")
			}
			seen[tuple] = struct{}{}
		}
	}
	return nil
}

func validateTuple(tuple Tuple) error {
	if strings.TrimSpace(tuple.User) == "" || strings.TrimSpace(tuple.Relation) == "" || strings.TrimSpace(tuple.Object) == "" {
		return fmt.Errorf("REBAC-CLIENT-VALIDATE-TUPLE tuple user, relation, and object must not be blank")
	}
	return nil
}

func (client *Client) storePath(parts ...string) *url.URL {
	path := strings.TrimSuffix(client.baseURL.Path, "/") + "/stores/" + url.PathEscape(client.config.StoreID)
	for _, part := range parts {
		path += "/" + url.PathEscape(part)
	}
	result := *client.baseURL
	result.Path = path
	return &result
}

func (client *Client) doJSON(ctx context.Context, method string, endpoint *url.URL, payload any, response any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("REBAC-CLIENT-ENCODE marshal OpenFGA request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return fmt.Errorf("REBAC-CLIENT-REQUEST create OpenFGA request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+client.config.Token)
	}
	httpResponse, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("REBAC-CLIENT-EXECUTE OpenFGA request failed: %w", err)
	}
	defer func() { _ = httpResponse.Body.Close() }()
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return openFGAStatusError(httpResponse)
	}
	if response == nil {
		return nil
	}
	content, err := io.ReadAll(io.LimitReader(httpResponse.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("REBAC-CLIENT-READ read OpenFGA response: %w", err)
	}
	if len(content) > maxResponseBytes {
		return fmt.Errorf("REBAC-CLIENT-READ OpenFGA response exceeds maximum size")
	}
	if err := json.Unmarshal(content, response); err != nil {
		return fmt.Errorf("REBAC-CLIENT-DECODE decode OpenFGA response: %w", err)
	}
	return nil
}

func (client *Client) doStream(ctx context.Context, endpoint *url.URL, payload any) (*http.Response, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-ENCODE marshal OpenFGA request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-REQUEST create OpenFGA request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	if client.config.Token != "" {
		request.Header.Set("Authorization", "Bearer "+client.config.Token)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-EXECUTE OpenFGA request failed: %w", err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		defer func() { _ = response.Body.Close() }()
		return nil, openFGAStatusError(response)
	}
	return response, nil
}

func openFGAStatusError(response *http.Response) error {
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err == nil && len(content) <= maxResponseBytes {
		var detail struct {
			Code string `json:"code"`
		}
		if json.Unmarshal(content, &detail) == nil && safeServerCode(detail.Code) {
			return fmt.Errorf("REBAC-CLIENT-STATUS OpenFGA returned HTTP status %d (%s)", response.StatusCode, detail.Code)
		}
	}
	return fmt.Errorf("REBAC-CLIENT-STATUS OpenFGA returned HTTP status %d", response.StatusCode)
}

func safeServerCode(code string) bool {
	if len(code) == 0 || len(code) > 64 {
		return false
	}
	for _, character := range code {
		if (character < 'a' || character > 'z') && character != '_' {
			return false
		}
	}
	return true
}

type tupleKeys struct {
	TupleKeys []Tuple `json:"tuple_keys"`
}

type listObjectsRequest struct {
	AuthorizationModelID string    `json:"authorization_model_id"`
	Consistency          string    `json:"consistency"`
	User                 string    `json:"user"`
	Relation             string    `json:"relation"`
	Type                 string    `json:"type"`
	ContextualTuples     tupleKeys `json:"contextual_tuples"`
}

type streamedListObjectsResponse struct {
	Result struct {
		Object string `json:"object"`
	} `json:"result"`
}

const higherConsistency = "HIGHER_CONSISTENCY"

type checkRequest struct {
	AuthorizationModelID string    `json:"authorization_model_id"`
	Consistency          string    `json:"consistency"`
	TupleKey             Tuple     `json:"tuple_key"`
	ContextualTuples     tupleKeys `json:"contextual_tuples"`
}
type checkResponse struct {
	Allowed *bool `json:"allowed"`
}
type batchCheckRequest struct {
	AuthorizationModelID string           `json:"authorization_model_id"`
	Consistency          string           `json:"consistency"`
	Checks               []batchCheckItem `json:"checks"`
}
type batchCheckItem struct {
	TupleKey         Tuple     `json:"tuple_key"`
	CorrelationID    string    `json:"correlation_id"`
	ContextualTuples tupleKeys `json:"contextual_tuples"`
}
type batchCheckResponse struct {
	Result map[string]batchCheckItemResponse `json:"result"`
}
type batchCheckItemResponse struct {
	Allowed *bool           `json:"allowed"`
	Error   json.RawMessage `json:"error,omitempty"`
}
type writeRequest struct {
	AuthorizationModelID string     `json:"authorization_model_id"`
	Writes               *tupleKeys `json:"writes,omitempty"`
	Deletes              *tupleKeys `json:"deletes,omitempty"`
}
type readTuplesResponse struct {
	ContinuationToken string `json:"continuation_token"`
	Tuples            []struct {
		Key Tuple `json:"key"`
	} `json:"tuples"`
}
type readTuplesRequest struct {
	TupleKey Tuple `json:"tuple_key"`
}
type readModelResponse struct {
	AuthorizationModel json.RawMessage `json:"authorization_model"`
}
type changesResponse struct {
	Changes           []changeResponse `json:"changes"`
	ContinuationToken string           `json:"continuation_token"`
}
type changeResponse struct {
	TupleKey  Tuple     `json:"tuple_key"`
	Operation string    `json:"operation"`
	Timestamp time.Time `json:"timestamp"`
}

// ReadTuples returns a high-consistency page for projection reconciliation.
func (client *Client) ReadTuples(ctx context.Context, continuation string) (tuples []Tuple, next string, err error) {
	measure := client.metrics.measure(ctx, "read_tuples", -1)
	defer func() { measure(err) }()
	ctx, span := otel.Tracer("github.com/eclipse-basyx/basyx-go-components/rebac").Start(ctx, "openfga.read_tuples")
	defer span.End()
	payload := struct {
		PageSize     int    `json:"page_size"`
		Continuation string `json:"continuation_token,omitempty"`
		Consistency  string `json:"consistency"`
	}{100, continuation, "HIGHER_CONSISTENCY"}
	var response readTuplesResponse
	if err = client.doJSON(ctx, http.MethodPost, client.storePath("read"), payload, &response); err != nil {
		return nil, "", recordSpanError(span, err)
	}
	for _, entry := range response.Tuples {
		if err = validateTuple(entry.Key); err != nil {
			return nil, "", err
		}
		tuples = append(tuples, entry.Key)
	}
	return tuples, response.ContinuationToken, nil
}
