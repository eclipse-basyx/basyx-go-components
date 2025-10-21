/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import (
	"encoding/json"
	"fmt"
)

// ModelTypeError indicates an unsupported model type was encountered
type ModelTypeError struct {
	Expected string
	Got      string
}

func (e *ModelTypeError) Error() string {
	return fmt.Sprintf("unsupported model type: expected %s, got %s", e.Expected, e.Got)
}

type EmbeddedDataSpecification struct {
	DataSpecificationContent DataSpecificationContent `json:"dataSpecificationContent"`

	DataSpecification *Reference `json:"dataSpecification"`
}

// UnmarshalJSON implements custom unmarshaling for EmbeddedDataSpecification
func (eds *EmbeddedDataSpecification) UnmarshalJSON(data []byte) error {
	type Alias EmbeddedDataSpecification
	aux := &struct {
		DataSpecificationContent json.RawMessage `json:"dataSpecificationContent"`
		DataSpecification        *Reference      `json:"dataSpecification"`
	}{}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Try to detect type first
	var rawMap map[string]interface{}
	if err := json.Unmarshal(aux.DataSpecificationContent, &rawMap); err != nil {
		return err
	}

	modelType, _ := rawMap["modelType"].(string)
	if modelType == "" {
		// Default to IEC61360 if no modelType specified
		modelType = "DataSpecificationIec61360"
	}

	switch modelType {
	case "DataSpecificationIec61360":
		var iec61360Content DataSpecificationIec61360
		if err := json.Unmarshal(aux.DataSpecificationContent, &iec61360Content); err != nil {
			return err // Return error instead of silently failing
		}
		eds.DataSpecificationContent = &iec61360Content
	default:
		return &ModelTypeError{Expected: "DataSpecificationIec61360", Got: modelType}
	}

	eds.DataSpecification = aux.DataSpecification
	return nil
}

// AssertEmbeddedDataSpecificationRequired checks if the required fields are not zero-ed
func AssertEmbeddedDataSpecificationRequired(obj EmbeddedDataSpecification) error {
	elements := map[string]interface{}{
		"dataSpecificationContent": obj.DataSpecificationContent,
		"dataSpecification":        obj.DataSpecification,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}
	if obj.DataSpecification != nil {
		if err := AssertReferenceRequired(*obj.DataSpecification); err != nil {
			return err
		}
	}
	return nil
}

// AssertEmbeddedDataSpecificationConstraints checks if the values respects the defined constraints
func AssertEmbeddedDataSpecificationConstraints(obj EmbeddedDataSpecification) error {
	if obj.DataSpecification != nil {
		if err := AssertReferenceConstraints(*obj.DataSpecification); err != nil {
			return err
		}
	}
	return nil
}
