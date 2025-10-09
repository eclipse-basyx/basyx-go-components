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
	"fmt"
)

type QualifierKind string

// List of QualifierKind
const (
	QUALIFIERKIND_CONCEPT_QUALIFIER  QualifierKind = "ConceptQualifier"
	QUALIFIERKIND_TEMPLATE_QUALIFIER QualifierKind = "TemplateQualifier"
	QUALIFIERKIND_VALUE_QUALIFIER    QualifierKind = "ValueQualifier"
)

// AllowedQualifierKindEnumValues is all the allowed values of QualifierKind enum
var AllowedQualifierKindEnumValues = []QualifierKind{
	"ConceptQualifier",
	"TemplateQualifier",
	"ValueQualifier",
}

// validQualifierKindEnumValue provides a map of QualifierKinds for fast verification of use input
var validQualifierKindEnumValues = map[QualifierKind]struct{}{
	"ConceptQualifier":  {},
	"TemplateQualifier": {},
	"ValueQualifier":    {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v QualifierKind) IsValid() bool {
	_, ok := validQualifierKindEnumValues[v]
	return ok
}

// NewQualifierKindFromValue returns a pointer to a valid QualifierKind
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewQualifierKindFromValue(v string) (QualifierKind, error) {
	ev := QualifierKind(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for QualifierKind: valid values are %v", v, AllowedQualifierKindEnumValues)
}

// AssertQualifierKindRequired checks if the required fields are not zero-ed
func AssertQualifierKindRequired(obj QualifierKind) error {
	return nil
}

// AssertQualifierKindConstraints checks if the values respects the defined constraints
func AssertQualifierKindConstraints(obj QualifierKind) error {
	return nil
}
