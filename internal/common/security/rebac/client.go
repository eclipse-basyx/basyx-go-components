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

package rebac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/telemetry"
	fgasdk "github.com/openfga/go-sdk"
	fgaclient "github.com/openfga/go-sdk/client"
	"github.com/openfga/go-sdk/credentials"
)

// maxWriteTuples is the chunk size of one OpenFGA write transaction.
const maxWriteTuples = 100

// ErrStoreNotFound reports that no store exists for a scope.
var ErrStoreNotFound = errors.New("REBAC-STORE-NOTFOUND OpenFGA store not found")

// CheckItem is one relationship check with its contextual tuples.
type CheckItem struct {
	User       string
	Relation   string
	Object     string
	Contextual []Tuple
}

// Client is the OpenFGA surface used by BaSyx. All calls are bounded by the
// configured timeout; errors must be treated as "cannot decide".
type Client interface {
	Check(ctx context.Context, item CheckItem) (bool, error)
	BatchCheck(ctx context.Context, items []CheckItem) ([]bool, error)
	ListObjects(ctx context.Context, user string, relation string, objectType string, contextual []Tuple) ([]string, error)
	Write(ctx context.Context, writes []Tuple, deletes []Tuple) error
	Read(ctx context.Context, object string, continuation string) ([]Tuple, string, error)
	ReadModel(ctx context.Context, modelID string) ([]byte, error)
}

// OpenFGAClient implements Client with the OpenFGA Go SDK over the
// instrumented BaSyx HTTP transport.
type OpenFGAClient struct {
	sdk          *fgaclient.OpenFgaClient
	storeID      string
	modelID      string
	timeout      time.Duration
	consistency  fgasdk.ConsistencyPreference
	maxBatchSize int32
}

// NewOpenFGAClient creates a client bound to one store and pinned model.
func NewOpenFGAClient(cfg common.ReBACOpenFGAConfig, storeID string, modelID string) (*OpenFGAClient, error) {
	sdk, err := newSDKClient(cfg, storeID, modelID)
	if err != nil {
		return nil, err
	}
	return &OpenFGAClient{
		sdk:          sdk,
		storeID:      storeID,
		modelID:      modelID,
		timeout:      time.Duration(cfg.TimeoutMillis) * time.Millisecond,
		consistency:  fgasdk.ConsistencyPreference(cfg.Consistency),
		maxBatchSize: int32(cfg.BatchCheckMaxItems), //nolint:gosec // validated to 1..50 by configuration
	}, nil
}

func newSDKClient(cfg common.ReBACOpenFGAConfig, storeID string, modelID string) (*fgaclient.OpenFgaClient, error) {
	creds, err := sdkCredentials(cfg.Credentials)
	if err != nil {
		return nil, err
	}
	sdk, err := fgaclient.NewSdkClient(&fgaclient.ClientConfiguration{
		ApiUrl:               cfg.APIURL,
		StoreId:              storeID,
		AuthorizationModelId: modelID,
		Credentials:          creds,
		HTTPClient:           &http.Client{Transport: telemetry.HTTPClientTransport(http.DefaultTransport)},
		RetryParams:          &fgasdk.RetryParams{MaxRetry: 1, MinWaitInMs: 50},
	})
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-CREATE: %w", err)
	}
	return sdk, nil
}

func sdkCredentials(cfg common.ReBACOpenFGACredentialsConfig) (*credentials.Credentials, error) {
	config := credentials.Credentials{Method: credentials.CredentialsMethodNone}
	switch cfg.Method {
	case common.ReBACCredentialsAPIToken:
		config = credentials.Credentials{
			Method: credentials.CredentialsMethodApiToken,
			Config: &credentials.Config{ApiToken: cfg.APIToken},
		}
	case common.ReBACCredentialsClientCredentials:
		config = credentials.Credentials{
			Method: credentials.CredentialsMethodClientCredentials,
			Config: &credentials.Config{
				ClientCredentialsClientId:       cfg.ClientID,
				ClientCredentialsClientSecret:   cfg.ClientSecret,
				ClientCredentialsApiTokenIssuer: cfg.APITokenIssuer,
				ClientCredentialsApiAudience:    cfg.APIAudience,
			},
		}
	}
	creds, err := credentials.NewCredentials(config)
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-CREDENTIALS: %w", err)
	}
	return creds, nil
}

func (c *OpenFGAClient) bounded(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

// Check evaluates one relationship.
func (c *OpenFGAClient) Check(ctx context.Context, item CheckItem) (bool, error) {
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	response, err := c.sdk.Check(ctx).Body(fgaclient.ClientCheckRequest{
		User:             item.User,
		Relation:         item.Relation,
		Object:           item.Object,
		ContextualTuples: sdkTuples(item.Contextual),
	}).Options(fgaclient.ClientCheckOptions{Consistency: &c.consistency}).Execute()
	if err != nil {
		return false, fmt.Errorf("REBAC-CLIENT-CHECK: %w", err)
	}
	return response.GetAllowed(), nil
}

// BatchCheck evaluates many relationships. Any per-item error fails the
// whole batch so callers never mistake an error for a deny or an allow.
func (c *OpenFGAClient) BatchCheck(ctx context.Context, items []CheckItem) ([]bool, error) {
	if len(items) == 0 {
		return nil, nil
	}
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	checks := make([]fgaclient.ClientBatchCheckItem, len(items))
	for index, item := range items {
		checks[index] = fgaclient.ClientBatchCheckItem{
			User:             item.User,
			Relation:         item.Relation,
			Object:           item.Object,
			CorrelationId:    strconv.Itoa(index),
			ContextualTuples: sdkTuples(item.Contextual),
		}
	}
	response, err := c.sdk.BatchCheck(ctx).Body(fgaclient.ClientBatchCheckRequest{Checks: checks}).
		Options(fgaclient.BatchCheckOptions{MaxBatchSize: &c.maxBatchSize, Consistency: &c.consistency}).Execute()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-BATCHCHECK: %w", err)
	}
	return batchCheckResults(response, len(items))
}

func batchCheckResults(response *fgasdk.BatchCheckResponse, count int) ([]bool, error) {
	results := response.GetResult()
	allowed := make([]bool, count)
	for index := range allowed {
		result, ok := results[strconv.Itoa(index)]
		if !ok || result.Error != nil || result.Allowed == nil {
			return nil, fmt.Errorf("REBAC-CLIENT-BATCHCHECK-ITEM check %d returned no decision", index)
		}
		allowed[index] = *result.Allowed
	}
	return allowed, nil
}

// ListObjects returns the objects of objectType the user holds relation on.
func (c *OpenFGAClient) ListObjects(ctx context.Context, user string, relation string, objectType string, contextual []Tuple) ([]string, error) {
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	response, err := c.sdk.ListObjects(ctx).Body(fgaclient.ClientListObjectsRequest{
		User:             user,
		Relation:         relation,
		Type:             objectType,
		ContextualTuples: sdkTuples(contextual),
	}).Options(fgaclient.ClientListObjectsOptions{Consistency: &c.consistency}).Execute()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-LISTOBJECTS: %w", err)
	}
	return response.GetObjects(), nil
}

// Write applies tuple writes and deletes idempotently in transactions of at
// most maxWriteTuples tuples. Existing writes and missing deletes succeed.
func (c *OpenFGAClient) Write(ctx context.Context, writes []Tuple, deletes []Tuple) error {
	for len(writes) > 0 || len(deletes) > 0 {
		chunkWrites, chunkDeletes := splitWriteChunk(writes, deletes)
		if err := c.writeChunk(ctx, chunkWrites, chunkDeletes); err != nil {
			return err
		}
		writes = writes[len(chunkWrites):]
		deletes = deletes[len(chunkDeletes):]
	}
	return nil
}

func splitWriteChunk(writes []Tuple, deletes []Tuple) ([]Tuple, []Tuple) {
	writeCount := min(len(writes), maxWriteTuples)
	deleteCount := min(len(deletes), maxWriteTuples-writeCount)
	return writes[:writeCount], deletes[:deleteCount]
}

func (c *OpenFGAClient) writeChunk(ctx context.Context, writes []Tuple, deletes []Tuple) error {
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	body := fgaclient.ClientWriteRequest{}
	for _, tuple := range writes {
		body.Writes = append(body.Writes, fgaclient.ClientTupleKey{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object})
	}
	for _, tuple := range deletes {
		body.Deletes = append(body.Deletes, fgaclient.ClientTupleKeyWithoutCondition{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object})
	}
	_, err := c.sdk.Write(ctx).Body(body).Options(fgaclient.ClientWriteOptions{
		Conflict: fgaclient.ClientWriteConflictOptions{
			OnDuplicateWrites: fgaclient.CLIENT_WRITE_REQUEST_ON_DUPLICATE_WRITES_IGNORE,
			OnMissingDeletes:  fgaclient.CLIENT_WRITE_REQUEST_ON_MISSING_DELETES_IGNORE,
		},
	}).Execute()
	if err != nil {
		return fmt.Errorf("REBAC-CLIENT-WRITE: %w", err)
	}
	return nil
}

// Read returns one page of stored tuples of an object, or of the whole store
// when object is empty.
func (c *OpenFGAClient) Read(ctx context.Context, object string, continuation string) ([]Tuple, string, error) {
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	pageSize := int32(100)
	options := fgaclient.ClientReadOptions{PageSize: &pageSize, Consistency: &c.consistency}
	if continuation != "" {
		options.ContinuationToken = &continuation
	}
	body := fgaclient.ClientReadRequest{}
	if object != "" {
		body.Object = &object
	}
	response, err := c.sdk.Read(ctx).Body(body).Options(options).Execute()
	if err != nil {
		return nil, "", fmt.Errorf("REBAC-CLIENT-READ: %w", err)
	}
	tuples := make([]Tuple, 0, len(response.GetTuples()))
	for _, tuple := range response.GetTuples() {
		key := tuple.GetKey()
		tuples = append(tuples, Tuple{User: key.GetUser(), Relation: key.GetRelation(), Object: key.GetObject()})
	}
	return tuples, response.GetContinuationToken(), nil
}

// ReadModel returns the JSON of a stored authorization model.
func (c *OpenFGAClient) ReadModel(ctx context.Context, modelID string) ([]byte, error) {
	ctx, cancel := c.bounded(ctx)
	defer cancel()
	response, err := c.sdk.ReadAuthorizationModel(ctx).
		Options(fgaclient.ClientReadAuthorizationModelOptions{AuthorizationModelId: &modelID}).Execute()
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-READMODEL: %w", err)
	}
	model, err := json.Marshal(response.GetAuthorizationModel())
	if err != nil {
		return nil, fmt.Errorf("REBAC-CLIENT-READMODEL-ENCODE: %w", err)
	}
	return model, nil
}

func sdkTuples(tuples []Tuple) []fgaclient.ClientContextualTupleKey {
	if len(tuples) == 0 {
		return nil
	}
	converted := make([]fgaclient.ClientContextualTupleKey, len(tuples))
	for index, tuple := range tuples {
		converted[index] = fgaclient.ClientContextualTupleKey{User: tuple.User, Relation: tuple.Relation, Object: tuple.Object}
	}
	return converted
}
