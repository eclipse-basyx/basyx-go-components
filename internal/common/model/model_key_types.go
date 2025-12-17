/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
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

/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](https://industrialdigitaltwin.org/en/content-hub/aasspecifications).   Copyright: Industrial Digital Twin Association (IDTA) 2025
 *
 * API version: V3.1.1_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"fmt"
)

// KeyTypes type of KeyTypes
type KeyTypes string

// List of KeyTypes
//
//nolint:all
const (
	KEYTYPES_ANNOTATED_RELATIONSHIP_ELEMENT KeyTypes = "AnnotatedRelationshipElement"
	KEYTYPES_ASSET_ADMINISTRATION_SHELL     KeyTypes = "AssetAdministrationShell"
	KEYTYPES_BASIC_EVENT_ELEMENT            KeyTypes = "BasicEventElement"
	KEYTYPES_BLOB                           KeyTypes = "Blob"
	KEYTYPES_CAPABILITY                     KeyTypes = "Capability"
	KEYTYPES_CONCEPT_DESCRIPTION            KeyTypes = "ConceptDescription"
	KEYTYPES_DATA_ELEMENT                   KeyTypes = "DataElement"
	KEYTYPES_ENTITY                         KeyTypes = "Entity"
	KEYTYPES_EVENT_ELEMENT                  KeyTypes = "EventElement"
	KEYTYPES_FILE                           KeyTypes = "File"
	KEYTYPES_FRAGMENT_REFERENCE             KeyTypes = "FragmentReference"
	KEYTYPES_GLOBAL_REFERENCE               KeyTypes = "GlobalReference"
	KEYTYPES_IDENTIFIABLE                   KeyTypes = "Identifiable"
	KEYTYPES_MULTI_LANGUAGE_PROPERTY        KeyTypes = "MultiLanguageProperty"
	KEYTYPES_OPERATION                      KeyTypes = "Operation"
	KEYTYPES_PROPERTY                       KeyTypes = "Property"
	KEYTYPES_RANGE                          KeyTypes = "Range"
	KEYTYPES_REFERABLE                      KeyTypes = "Referable"
	KEYTYPES_REFERENCE_ELEMENT              KeyTypes = "ReferenceElement"
	KEYTYPES_RELATIONSHIP_ELEMENT           KeyTypes = "RelationshipElement"
	KEYTYPES_SUBMODEL                       KeyTypes = "Submodel"
	KEYTYPES_SUBMODEL_ELEMENT               KeyTypes = "SubmodelElement"
	KEYTYPES_SUBMODEL_ELEMENT_COLLECTION    KeyTypes = "SubmodelElementCollection"
	KEYTYPES_SUBMODEL_ELEMENT_LIST          KeyTypes = "SubmodelElementList"
)

// AllowedKeyTypesEnumValues is all the allowed values of KeyTypes enum
var AllowedKeyTypesEnumValues = []KeyTypes{
	"AnnotatedRelationshipElement",
	"AssetAdministrationShell",
	"BasicEventElement",
	"Blob",
	"Capability",
	"ConceptDescription",
	"DataElement",
	"Entity",
	"EventElement",
	"File",
	"FragmentReference",
	"GlobalReference",
	"Identifiable",
	"MultiLanguageProperty",
	"Operation",
	"Property",
	"Range",
	"Referable",
	"ReferenceElement",
	"RelationshipElement",
	"Submodel",
	"SubmodelElement",
	"SubmodelElementCollection",
	"SubmodelElementList",
}

// validKeyTypesEnumValue provides a map of KeyTypess for fast verification of use input
var validKeyTypesEnumValues = map[KeyTypes]struct{}{
	"AnnotatedRelationshipElement": {},
	"AssetAdministrationShell":     {},
	"BasicEventElement":            {},
	"Blob":                         {},
	"Capability":                   {},
	"ConceptDescription":           {},
	"DataElement":                  {},
	"Entity":                       {},
	"EventElement":                 {},
	"File":                         {},
	"FragmentReference":            {},
	"GlobalReference":              {},
	"Identifiable":                 {},
	"MultiLanguageProperty":        {},
	"Operation":                    {},
	"Property":                     {},
	"Range":                        {},
	"Referable":                    {},
	"ReferenceElement":             {},
	"RelationshipElement":          {},
	"Submodel":                     {},
	"SubmodelElement":              {},
	"SubmodelElementCollection":    {},
	"SubmodelElementList":          {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v KeyTypes) IsValid() bool {
	_, ok := validKeyTypesEnumValues[v]
	return ok
}

// NewKeyTypesFromValue returns a pointer to a valid KeyTypes
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewKeyTypesFromValue(v string) (KeyTypes, error) {
	ev := KeyTypes(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for KeyTypes: valid values are %v", v, AllowedKeyTypesEnumValues)
}

// AssertKeyTypesRequired checks if the required fields are not zero-ed
func AssertKeyTypesRequired(_ KeyTypes) error {
	return nil
}

// AssertKeyTypesConstraints checks if the values respects the defined constraints
func AssertKeyTypesConstraints(_ KeyTypes) error {
	return nil
}
