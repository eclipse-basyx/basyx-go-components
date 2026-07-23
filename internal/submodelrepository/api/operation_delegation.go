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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func parseDelegationTimeout(clientTimeoutDuration string) (time.Duration, error) {
	if strings.TrimSpace(clientTimeoutDuration) == "" {
		return defaultDelegationTimeout, nil
	}

	trimmed := strings.TrimSpace(clientTimeoutDuration)
	if !strings.HasPrefix(trimmed, "P") && !strings.HasPrefix(trimmed, "-P") {
		return 0, fmt.Errorf("SMREPO-PARSETO-INVALID clientTimeoutDuration '%s' is not an ISO8601 duration", clientTimeoutDuration)
	}

	sign := 1.0
	if strings.HasPrefix(trimmed, "-P") {
		sign = -1.0
		trimmed = strings.TrimPrefix(trimmed, "-")
	}

	trimmed = strings.TrimPrefix(trimmed, "P")
	parts := strings.SplitN(trimmed, "T", 2)
	datePart := parts[0]
	timePart := ""
	if len(parts) == 2 {
		timePart = parts[1]
	}

	if strings.Contains(datePart, "Y") || strings.Contains(datePart, "M") {
		return 0, fmt.Errorf("SMREPO-PARSETO-UNSUPPORTED years and months are not supported in clientTimeoutDuration")
	}

	totalDuration, err := parseDelegationDateDuration(datePart)
	if err != nil {
		return 0, err
	}

	timeDuration, err := parseDelegationTimeDuration(timePart)
	if err != nil {
		return 0, err
	}
	totalDuration += timeDuration

	computedDuration := time.Duration(float64(totalDuration) * sign)
	if computedDuration <= 0 {
		return 0, errors.New("SMREPO-PARSETO-NONPOSITIVE clientTimeoutDuration must resolve to a positive duration")
	}

	return computedDuration, nil
}

func parseDelegationDateDuration(datePart string) (time.Duration, error) {
	remainingDate := datePart
	var totalDuration time.Duration
	if strings.Contains(remainingDate, "D") {
		dayParts := strings.SplitN(remainingDate, "D", 2)
		days, err := strconv.Atoi(dayParts[0])
		if err != nil {
			return 0, fmt.Errorf("SMREPO-PARSETO-PARSEDAYS %w", err)
		}
		totalDuration += time.Duration(days) * 24 * time.Hour
		remainingDate = dayParts[1]
	}

	if remainingDate != "" {
		return 0, fmt.Errorf("SMREPO-PARSETO-INVALIDDATE unsupported date part '%s'", remainingDate)
	}

	return totalDuration, nil
}

func parseDelegationTimeDuration(timePart string) (time.Duration, error) {
	remainingTime := timePart
	var totalDuration time.Duration

	hourDuration, rest, err := parseDelegationIntTimePart(remainingTime, "H", time.Hour, "SMREPO-PARSETO-PARSEHOURS")
	if err != nil {
		return 0, err
	}
	totalDuration += hourDuration
	remainingTime = rest

	minuteDuration, rest, err := parseDelegationIntTimePart(remainingTime, "M", time.Minute, "SMREPO-PARSETO-PARSEMINUTES")
	if err != nil {
		return 0, err
	}
	totalDuration += minuteDuration
	remainingTime = rest

	secondDuration, rest, err := parseDelegationSecondsPart(remainingTime)
	if err != nil {
		return 0, err
	}
	totalDuration += secondDuration
	remainingTime = rest

	if strings.TrimSpace(remainingTime) != "" {
		return 0, fmt.Errorf("SMREPO-PARSETO-INVALIDTIME unsupported time part '%s'", remainingTime)
	}

	return totalDuration, nil
}

func parseDelegationIntTimePart(remainingTime string, suffix string, unit time.Duration, code string) (time.Duration, string, error) {
	if !strings.Contains(remainingTime, suffix) {
		return 0, remainingTime, nil
	}

	parts := strings.SplitN(remainingTime, suffix, 2)
	value, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("%s %w", code, err)
	}

	return time.Duration(value) * unit, parts[1], nil
}

func parseDelegationSecondsPart(remainingTime string) (time.Duration, string, error) {
	if !strings.Contains(remainingTime, "S") {
		return 0, remainingTime, nil
	}

	secondsParts := strings.SplitN(remainingTime, "S", 2)
	seconds, err := strconv.ParseFloat(secondsParts[0], 64)
	if err != nil {
		return 0, "", fmt.Errorf("SMREPO-PARSETO-PARSESECONDS %w", err)
	}

	return time.Duration(seconds * float64(time.Second)), secondsParts[1], nil
}

func resolveDelegationURL(element types.ISubmodelElement) (string, error) {
	if element == nil {
		return "", errors.New("SMREPO-RSLVDEL-NILELEMENT submodel element is nil")
	}

	if element.ModelType() != types.ModelTypeOperation {
		return "", common.NewErrBadRequest("invoke is only valid for Operation submodel elements")
	}

	for _, qualifier := range element.Qualifiers() {
		if qualifier == nil {
			continue
		}
		if qualifier.Type() == invocationDelegationQualifierType && qualifier.Value() != nil {
			delegationTarget := strings.TrimSpace(*qualifier.Value())
			if delegationTarget == "" {
				return "", errors.New("SMREPO-RSLVDEL-EMPTYURL invocationDelegation qualifier value is empty")
			}
			return delegationTarget, nil
		}
	}

	return "", errors.New("SMREPO-RSLVDEL-MISSINGQUAL invocationDelegation qualifier not found on operation")
}

func buildDelegatedOperationInput(operationRequest gen.OperationRequest) []types.IOperationVariable {
	delegatedInput := make([]types.IOperationVariable, 0, len(operationRequest.InputArguments)+len(operationRequest.InoutputArguments))
	delegatedInput = append(delegatedInput, operationRequest.InputArguments...)
	delegatedInput = append(delegatedInput, operationRequest.InoutputArguments...)
	return delegatedInput
}

func serializeDelegatedOperationPayload(payload []types.IOperationVariable) ([]byte, error) {
	jsonablePayload := make([]any, 0, len(payload))
	for index, operationVariable := range payload {
		jsonableValue, err := jsonization.ToJsonable(operationVariable)
		if err != nil {
			return nil, fmt.Errorf("SMREPO-SERDEL-TOJSONABLE-%d %w", index, err)
		}
		jsonablePayload = append(jsonablePayload, jsonableValue)
	}

	requestBody, err := json.Marshal(jsonablePayload)
	if err != nil {
		return nil, fmt.Errorf("SMREPO-SERDEL-MARSHAL %w", err)
	}

	return requestBody, nil
}

func toDelegatedOperationResultPayload(outputArguments []any, inoutputArguments []any) map[string]any {
	return map[string]any{
		"executionState":    "Completed",
		"success":           true,
		"outputArguments":   outputArguments,
		"inoutputArguments": inoutputArguments,
	}
}

func toJsonableOperationVariable(item any) (any, bool) {
	operationVariable, err := jsonization.OperationVariableFromJsonable(item)
	if err != nil {
		return nil, false
	}
	jsonableOperationVariable, err := jsonization.ToJsonable(operationVariable)
	if err != nil {
		return nil, false
	}
	return jsonableOperationVariable, true
}

func jsonableOperationVariablesFromArray(payload any) ([]any, bool) {
	switch typedPayload := payload.(type) {
	case []types.IOperationVariable:
		return jsonableOperationVariablesFromTyped(typedPayload)
	case []any:
		return jsonableOperationVariablesFromItems(typedPayload)
	case []map[string]any:
		items := make([]any, 0, len(typedPayload))
		for _, item := range typedPayload {
			items = append(items, item)
		}
		return jsonableOperationVariablesFromItems(items)
	default:
		return nil, false
	}
}

func jsonableOperationVariablesFromTyped(payload []types.IOperationVariable) ([]any, bool) {
	jsonables := make([]any, 0, len(payload))
	for _, operationVariable := range payload {
		if operationVariable == nil {
			continue
		}
		jsonable, err := jsonization.ToJsonable(operationVariable)
		if err != nil {
			return nil, false
		}
		jsonables = append(jsonables, jsonable)
	}
	return jsonables, true
}

func jsonableOperationVariablesFromItems(items []any) ([]any, bool) {
	jsonables := make([]any, 0, len(items))
	for _, item := range items {
		jsonable, ok := toJsonableOperationVariable(item)
		if !ok {
			return nil, false
		}
		jsonables = append(jsonables, jsonable)
	}
	return jsonables, true
}

func toDelegatedOperationResultPayloadFromBody(delegatedBody any) (map[string]any, bool) {
	if outputArguments, ok := jsonableOperationVariablesFromArray(delegatedBody); ok {
		return toDelegatedOperationResultPayload(outputArguments, []any{}), true
	}

	delegatedBodyMap, ok := delegatedBody.(map[string]any)
	if !ok {
		return nil, false
	}

	outputArguments, outputOK := jsonableOperationVariablesFromArray(delegatedBodyMap["outputArguments"])
	inoutputArguments, inoutputOK := jsonableOperationVariablesFromArray(delegatedBodyMap["inoutputArguments"])
	if !outputOK && !inoutputOK {
		return nil, false
	}

	if !outputOK {
		outputArguments = []any{}
	}
	if !inoutputOK {
		inoutputArguments = []any{}
	}

	return toDelegatedOperationResultPayload(outputArguments, inoutputArguments), true
}

func parseDelegationAsyncTTL() time.Duration {
	rawTTL := strings.TrimSpace(os.Getenv(delegationAsyncTTLKey))
	if rawTTL == "" {
		return defaultDelegationAsyncTTL
	}

	parsedTTL, err := time.ParseDuration(rawTTL)
	if err != nil || parsedTTL <= 0 {
		return defaultDelegationAsyncTTL
	}

	return parsedTTL
}

func doDelegatedOperationCall(ctx context.Context, delegationURL string, payload []types.IOperationVariable, timeout time.Duration) (int, any, error) {
	parsedDelegationURL, parseErr := url.Parse(delegationURL)
	if parseErr != nil {
		return 0, nil, fmt.Errorf("SMREPO-DOOPDELG-PARSEURL %w", parseErr)
	}
	if parsedDelegationURL.Scheme != "http" && parsedDelegationURL.Scheme != "https" {
		return 0, nil, errors.New("SMREPO-DOOPDELG-UNSUPPORTEDSCHEME delegation URL must use http or https")
	}
	if strings.TrimSpace(parsedDelegationURL.Host) == "" {
		return 0, nil, errors.New("SMREPO-DOOPDELG-MISSINGHOST delegation URL host is missing")
	}

	return doTrustedDelegatedOperationCall(ctx, delegationURL, payload, timeout, newDelegationAddressGuard(nil, nil))
}

func doTrustedDelegatedOperationCall(
	ctx context.Context,
	delegationURL string,
	payload []types.IOperationVariable,
	timeout time.Duration,
	delegationGuard delegationAddressGuard,
) (int, any, error) {
	requestBody, marshalErr := serializeDelegatedOperationPayload(payload)
	if marshalErr != nil {
		return 0, nil, fmt.Errorf("SMREPO-DOOPDELG-MARSHALREQ %w", marshalErr)
	}

	request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, delegationURL, bytes.NewReader(requestBody))
	if requestErr != nil {
		return 0, nil, fmt.Errorf("SMREPO-DOOPDELG-CREATEREQ %w", requestErr)
	}

	request.Header.Set("Content-Type", "application/json")
	authorizationHeader := common.AuthorizationHeaderFromContext(ctx)
	if strings.TrimSpace(authorizationHeader) != "" {
		request.Header.Set("Authorization", authorizationHeader)
	}

	httpClient := newDelegationHTTPClient(timeout, delegationGuard)
	// #nosec G107,G704 -- delegation requests use a guarded transport that allowlists and pins the resolved IP before dialing.
	response, responseErr := httpClient.Do(request)
	if responseErr != nil {
		return 0, nil, fmt.Errorf("SMREPO-DOOPDELG-EXECREQ %w", responseErr)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	return readDelegatedOperationResponse(response)
}

func readDelegatedOperationResponse(response *http.Response) (int, any, error) {
	responseBytes, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return 0, nil, fmt.Errorf("SMREPO-DOOPDELG-READRESP %w", readErr)
	}

	if len(responseBytes) == 0 {
		return response.StatusCode, []any{}, nil
	}

	var responseBody any
	if unmarshalErr := json.Unmarshal(responseBytes, &responseBody); unmarshalErr != nil {
		return response.StatusCode, map[string]any{"message": string(responseBytes)}, nil
	}

	return response.StatusCode, responseBody, nil
}
