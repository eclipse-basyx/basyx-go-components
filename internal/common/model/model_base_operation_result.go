/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// BaseOperationResult type of OperationResult
type BaseOperationResult struct {
	Messages []Message `json:"messages,omitempty"`

	ExecutionState ExecutionState `json:"executionState,omitempty"`

	Success bool `json:"success,omitempty"`
}

// AssertBaseOperationResultRequired checks if the required fields are not zero-ed
func AssertBaseOperationResultRequired(obj BaseOperationResult) error {
	for _, el := range obj.Messages {
		if err := AssertMessageRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertBaseOperationResultConstraints checks if the values respects the defined constraints
func AssertBaseOperationResultConstraints(obj BaseOperationResult) error {
	for _, el := range obj.Messages {
		if err := AssertMessageConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
