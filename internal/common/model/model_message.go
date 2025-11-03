/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// Message type of Message
type Message struct {
	Code string `json:"code,omitempty"`

	CorrelationID string `json:"correlationID,omitempty"`

	MessageType string `json:"messageType,omitempty"`

	Text string `json:"text,omitempty"`

	Timestamp string `json:"timestamp,omitempty" validate:"regexp=^-?(([1-9][0-9][0-9][0-9]+)|(0[0-9][0-9][0-9]))-((0[1-9])|(1[0-2]))-((0[1-9])|([12][0-9])|(3[01]))T(((([01][0-9])|(2[0-3])):[0-5][0-9]:([0-5][0-9])(\\\\.[0-9]+)?)|24:00:00(\\\\.0+)?)(Z|\\\\+00:00|-00:00)$"`
}

// AssertMessageRequired checks if the required fields are not zero-ed
//
//nolint:all
func AssertMessageRequired(obj Message) error {
	return nil
}

// AssertMessageConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertMessageConstraints(obj Message) error {
	return nil
}
