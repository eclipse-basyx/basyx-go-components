/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"fmt"
)

type AasSubmodelElements string

// List of AasSubmodelElements
const (
	AASSUBMODELELEMENTS_ANNOTATED_RELATIONSHIP_ELEMENT AasSubmodelElements = "AnnotatedRelationshipElement"
	AASSUBMODELELEMENTS_BASIC_EVENT_ELEMENT            AasSubmodelElements = "BasicEventElement"
	AASSUBMODELELEMENTS_BLOB                           AasSubmodelElements = "Blob"
	AASSUBMODELELEMENTS_CAPABILITY                     AasSubmodelElements = "Capability"
	AASSUBMODELELEMENTS_DATA_ELEMENT                   AasSubmodelElements = "DataElement"
	AASSUBMODELELEMENTS_ENTITY                         AasSubmodelElements = "Entity"
	AASSUBMODELELEMENTS_EVENT_ELEMENT                  AasSubmodelElements = "EventElement"
	AASSUBMODELELEMENTS_FILE                           AasSubmodelElements = "File"
	AASSUBMODELELEMENTS_MULTI_LANGUAGE_PROPERTY        AasSubmodelElements = "MultiLanguageProperty"
	AASSUBMODELELEMENTS_OPERATION                      AasSubmodelElements = "Operation"
	AASSUBMODELELEMENTS_PROPERTY                       AasSubmodelElements = "Property"
	AASSUBMODELELEMENTS_RANGE                          AasSubmodelElements = "Range"
	AASSUBMODELELEMENTS_REFERENCE_ELEMENT              AasSubmodelElements = "ReferenceElement"
	AASSUBMODELELEMENTS_RELATIONSHIP_ELEMENT           AasSubmodelElements = "RelationshipElement"
	AASSUBMODELELEMENTS_SUBMODEL_ELEMENT               AasSubmodelElements = "SubmodelElement"
	AASSUBMODELELEMENTS_SUBMODEL_ELEMENT_COLLECTION    AasSubmodelElements = "SubmodelElementCollection"
	AASSUBMODELELEMENTS_SUBMODEL_ELEMENT_LIST          AasSubmodelElements = "SubmodelElementList"
)

// AllowedAasSubmodelElementsEnumValues is all the allowed values of AasSubmodelElements enum
var AllowedAasSubmodelElementsEnumValues = []AasSubmodelElements{
	"AnnotatedRelationshipElement",
	"BasicEventElement",
	"Blob",
	"Capability",
	"DataElement",
	"Entity",
	"EventElement",
	"File",
	"MultiLanguageProperty",
	"Operation",
	"Property",
	"Range",
	"ReferenceElement",
	"RelationshipElement",
	"SubmodelElement",
	"SubmodelElementCollection",
	"SubmodelElementList",
}

// validAasSubmodelElementsEnumValue provides a map of AasSubmodelElementss for fast verification of use input
var validAasSubmodelElementsEnumValues = map[AasSubmodelElements]struct{}{
	"AnnotatedRelationshipElement": {},
	"BasicEventElement":            {},
	"Blob":                         {},
	"Capability":                   {},
	"DataElement":                  {},
	"Entity":                       {},
	"EventElement":                 {},
	"File":                         {},
	"MultiLanguageProperty":        {},
	"Operation":                    {},
	"Property":                     {},
	"Range":                        {},
	"ReferenceElement":             {},
	"RelationshipElement":          {},
	"SubmodelElement":              {},
	"SubmodelElementCollection":    {},
	"SubmodelElementList":          {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v AasSubmodelElements) IsValid() bool {
	_, ok := validAasSubmodelElementsEnumValues[v]
	return ok
}

// NewAasSubmodelElementsFromValue returns a pointer to a valid AasSubmodelElements
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewAasSubmodelElementsFromValue(v string) (AasSubmodelElements, error) {
	ev := AasSubmodelElements(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for AasSubmodelElements: valid values are %v", v, AllowedAasSubmodelElementsEnumValues)
}

// AssertAasSubmodelElementsRequired checks if the required fields are not zero-ed
func AssertAasSubmodelElementsRequired(obj AasSubmodelElements) error {
	return nil
}

// AssertAasSubmodelElementsConstraints checks if the values respects the defined constraints
func AssertAasSubmodelElementsConstraints(obj AasSubmodelElements) error {
	return nil
}
