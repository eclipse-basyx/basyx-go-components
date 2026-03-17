package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type operationVariable struct {
	Value propertyValue `json:"value"`
}

type propertyValue struct {
	ModelType string `json:"modelType"`
	IDShort   string `json:"idShort"`
	ValueType string `json:"valueType"`
	Value     string `json:"value"`
}

type operationRequest struct {
	InputArguments []operationVariable `json:"inputArguments"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/delegate/add/sync", syncAddHandler)
	mux.HandleFunc("/delegate/add/async", asyncAddHandler)

	address := ":" + port
	log.Printf("delegated operation service listening on %s", address)
	if err := http.ListenAndServe(address, mux); err != nil {
		log.Fatalf("delegate service startup failed: %v", err)
	}
}

func healthHandler(writer http.ResponseWriter, _ *http.Request) {
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte("ok"))
}

func syncAddHandler(writer http.ResponseWriter, request *http.Request) {
	handleAdd(writer, request, 0)
}

func asyncAddHandler(writer http.ResponseWriter, request *http.Request) {
	handleAdd(writer, request, 2*time.Second)
}

func handleAdd(writer http.ResponseWriter, request *http.Request, delay time.Duration) {
	if request.Method != http.MethodPost {
		writeError(writer, "DELEGATE-HANDLEADD-METHOD method not allowed", http.StatusMethodNotAllowed)
		return
	}

	inputVariables, err := parseInput(request)
	if err != nil {
		writeError(writer, err.Error(), http.StatusBadRequest)
		return
	}

	a, parseErr := readIntByIDShort(inputVariables, "numberA")
	if parseErr != nil {
		writeError(writer, parseErr.Error(), http.StatusBadRequest)
		return
	}

	b, parseErr := readIntByIDShort(inputVariables, "numberB")
	if parseErr != nil {
		writeError(writer, parseErr.Error(), http.StatusBadRequest)
		return
	}

	if delay > 0 {
		time.Sleep(delay)
	}

	result := []operationVariable{
		{
			Value: propertyValue{
				ModelType: "Property",
				IDShort:   "sum",
				ValueType: "xs:int",
				Value:     strconv.Itoa(a + b),
			},
		},
	}

	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	if err = json.NewEncoder(writer).Encode(result); err != nil {
		writeError(writer, fmt.Sprintf("DELEGATE-HANDLEADD-ENCODE %v", err), http.StatusInternalServerError)
	}
}

func parseInput(request *http.Request) ([]operationVariable, error) {
	defer func() {
		_ = request.Body.Close()
	}()

	requestBytes, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("DELEGATE-PARSEINPUT-READ %w", err)
	}

	var payload any
	if err = json.Unmarshal(requestBytes, &payload); err != nil {
		return nil, fmt.Errorf("DELEGATE-PARSEINPUT-DECODE %w", err)
	}

	inputVariables, parseErr := extractOperationVariables(payload)
	if parseErr != nil {
		return nil, parseErr
	}

	if len(inputVariables) == 0 {
		return nil, errors.New("DELEGATE-PARSEINPUT-EMPTY no operation variables found")
	}

	return inputVariables, nil
}

func extractOperationVariables(payload any) ([]operationVariable, error) {
	switch typed := payload.(type) {
	case []any:
		return extractFromArray(typed)
	case map[string]any:
		if inputArgumentsRaw, found := getMapValueIgnoreCase(typed, "inputArguments"); found {
			return extractOperationVariables(inputArgumentsRaw)
		}

		if _, hasValue := getMapValueIgnoreCase(typed, "value"); hasValue {
			operationVariableItem, ok := toOperationVariable(typed)
			if !ok {
				return nil, errors.New("DELEGATE-EXTRACTOBJ-INVALID operation variable object is invalid")
			}
			return []operationVariable{operationVariableItem}, nil
		}

		return extractFromMap(typed)
	default:
		return nil, errors.New("DELEGATE-EXTRACT-UNSUPPORTED unsupported payload type")
	}
}

func extractFromArray(items []any) ([]operationVariable, error) {
	result := make([]operationVariable, 0, len(items))
	for _, item := range items {
		itemAsMap, ok := item.(map[string]any)
		if !ok {
			continue
		}

		operationVariableItem, variableOK := toOperationVariable(itemAsMap)
		if !variableOK {
			continue
		}

		result = append(result, operationVariableItem)
	}

	if len(result) == 0 {
		return nil, errors.New("DELEGATE-EXTRACTARR-NONE no operation variables could be parsed from array")
	}

	return result, nil
}

func extractFromMap(items map[string]any) ([]operationVariable, error) {
	result := make([]operationVariable, 0, len(items))
	for key, value := range items {
		propertyMap, ok := value.(map[string]any)
		if !ok {
			continue
		}

		idShort := getStringValue(propertyMap, "idShort", "IDShort")
		if idShort == "" {
			idShort = key
		}

		operationVariableItem, variableOK := toOperationVariable(map[string]any{
			"value": mergeMissingPropertyFields(propertyMap, idShort),
		})
		if !variableOK {
			continue
		}

		result = append(result, operationVariableItem)
	}

	if len(result) == 0 {
		return nil, errors.New("DELEGATE-EXTRACTMAP-NONE no operation variables could be parsed from object")
	}

	return result, nil
}

func toOperationVariable(payload map[string]any) (operationVariable, bool) {
	valueRaw, found := getMapValueIgnoreCase(payload, "value")
	if !found {
		return operationVariable{}, false
	}

	valueMap, ok := valueRaw.(map[string]any)
	if !ok {
		return operationVariable{}, false
	}

	property := propertyValue{
		ModelType: getStringValue(valueMap, "modelType", "ModelType"),
		IDShort:   getStringValue(valueMap, "idShort", "IDShort"),
		ValueType: getStringValue(valueMap, "valueType", "ValueType"),
		Value:     getAnyAsString(valueMap, "value", "Value"),
	}

	if property.ModelType == "" {
		property.ModelType = "Property"
	}
	if property.ValueType == "" {
		property.ValueType = "xs:string"
	}

	return operationVariable{Value: property}, true
}

func mergeMissingPropertyFields(propertyMap map[string]any, idShort string) map[string]any {
	merged := make(map[string]any, len(propertyMap)+3)
	for key, value := range propertyMap {
		merged[key] = value
	}

	if getStringValue(merged, "idShort", "IDShort") == "" {
		merged["idShort"] = idShort
	}
	if getStringValue(merged, "modelType", "ModelType") == "" {
		merged["modelType"] = "Property"
	}
	if getStringValue(merged, "valueType", "ValueType") == "" {
		merged["valueType"] = "xs:string"
	}

	return merged
}

func getMapValueIgnoreCase(m map[string]any, keys ...string) (any, bool) {
	for _, wantedKey := range keys {
		for existingKey, value := range m {
			if strings.EqualFold(existingKey, wantedKey) {
				return value, true
			}
		}
	}
	return nil, false
}

func getStringValue(m map[string]any, keys ...string) string {
	value := getAnyAsString(m, keys...)
	if value == "<nil>" {
		return ""
	}
	return value
}

func getAnyAsString(m map[string]any, keys ...string) string {
	value, found := getMapValueIgnoreCase(m, keys...)
	if !found {
		return ""
	}
	return fmt.Sprint(value)
}

func readIntByIDShort(inputVariables []operationVariable, idShort string) (int, error) {
	for _, operationVariableItem := range inputVariables {
		if operationVariableItem.Value.IDShort != idShort {
			continue
		}

		if operationVariableItem.Value.Value == "" {
			return 0, fmt.Errorf("DELEGATE-READINT-EMPTYVALUE value for %s is empty", idShort)
		}

		parsedNumber, err := strconv.Atoi(operationVariableItem.Value.Value)
		if err != nil {
			return 0, fmt.Errorf("DELEGATE-READINT-PARSEINT cannot parse %s as int: %w", idShort, err)
		}

		return parsedNumber, nil
	}

	fallbackValues := extractNumericValues(inputVariables)
	if idShort == "numberA" && len(fallbackValues) >= 1 {
		return fallbackValues[0], nil
	}
	if idShort == "numberB" && len(fallbackValues) >= 2 {
		return fallbackValues[1], nil
	}

	return 0, fmt.Errorf("DELEGATE-READINT-MISSING missing input variable %s", idShort)
}

func extractNumericValues(inputVariables []operationVariable) []int {
	result := make([]int, 0, len(inputVariables))
	for _, operationVariableItem := range inputVariables {
		if operationVariableItem.Value.Value == "" {
			continue
		}

		parsedNumber, err := strconv.Atoi(operationVariableItem.Value.Value)
		if err != nil {
			continue
		}

		result = append(result, parsedNumber)
	}

	return result
}

func writeError(writer http.ResponseWriter, message string, statusCode int) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_ = json.NewEncoder(writer).Encode(map[string]string{"message": message})
}
