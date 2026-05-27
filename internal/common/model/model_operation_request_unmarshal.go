package model

import (
	"encoding/json"
	"fmt"

	"github.com/aas-core-works/aas-core3.1-golang/jsonization"
	"github.com/aas-core-works/aas-core3.1-golang/types"
)

type operationRequestJSON struct {
	InoutputArguments     json.RawMessage `json:"inoutputArguments,omitempty"`
	InputArguments        json.RawMessage `json:"inputArguments,omitempty"`
	ClientTimeoutDuration string          `json:"clientTimeoutDuration,omitempty"`
}

// UnmarshalJSON decodes operation request arguments in array/object forms and preserves timeout settings.
func (o *OperationRequest) UnmarshalJSON(data []byte) error {
	var raw operationRequestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	inputArguments, err := parseOperationVariablesRaw(raw.InputArguments)
	if err != nil {
		return err
	}

	inoutputArguments, err := parseOperationVariablesRaw(raw.InoutputArguments)
	if err != nil {
		return err
	}

	o.InputArguments = inputArguments
	o.InoutputArguments = inoutputArguments
	o.ClientTimeoutDuration = raw.ClientTimeoutDuration

	return nil
}

func parseOperationVariablesRaw(rawValue json.RawMessage) ([]types.IOperationVariable, error) {
	if len(rawValue) == 0 {
		return nil, nil
	}

	var jsonable any
	if err := json.Unmarshal(rawValue, &jsonable); err != nil {
		return nil, err
	}

	switch typed := jsonable.(type) {
	case nil:
		return nil, nil
	case []any:
		return parseOperationVariablesFromArray(typed)
	case map[string]any:
		return parseOperationVariablesFromObject(typed)
	default:
		return nil, fmt.Errorf("SMREPO-OPREQ-INVALIDARGS unsupported operation variable payload type %T", typed)
	}
}

func parseOperationVariablesFromArray(items []any) ([]types.IOperationVariable, error) {
	result := make([]types.IOperationVariable, 0, len(items))
	for _, item := range items {
		operationVariable, err := jsonization.OperationVariableFromJsonable(item)
		if err != nil {
			return nil, err
		}
		result = append(result, operationVariable)
	}
	return result, nil
}

func parseOperationVariablesFromObject(payload map[string]any) ([]types.IOperationVariable, error) {
	if _, hasValue := payload["value"]; hasValue {
		operationVariable, err := jsonization.OperationVariableFromJsonable(payload)
		if err != nil {
			return nil, err
		}
		return []types.IOperationVariable{operationVariable}, nil
	}

	result := make([]types.IOperationVariable, 0, len(payload))
	for argumentName, argumentValue := range payload {
		operationVariableJSON := map[string]any{
			"value": ensureSubmodelElementValue(argumentName, argumentValue),
		}

		operationVariable, err := jsonization.OperationVariableFromJsonable(operationVariableJSON)
		if err != nil {
			return nil, err
		}
		result = append(result, operationVariable)
	}

	return result, nil
}

func ensureSubmodelElementValue(idShort string, value any) any {
	valueMap, ok := value.(map[string]any)
	if ok {
		if _, hasModelType := valueMap["modelType"]; hasModelType {
			if _, hasIDShort := valueMap["idShort"]; !hasIDShort {
				valueMap["idShort"] = idShort
			}
			if _, hasValueType := valueMap["valueType"]; !hasValueType {
				valueMap["valueType"] = "xs:string"
			}
			if _, hasValue := valueMap["value"]; !hasValue {
				valueMap["value"] = ""
			}
			return valueMap
		}
	}

	return map[string]any{
		"modelType": "Property",
		"idShort":   idShort,
		"valueType": "xs:string",
		"value":     fmt.Sprint(value),
	}
}
