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

package common

import "github.com/FriedJannik/aas-go-sdk/stringification"

type submodelElementMetadataFieldValidator func(raw any, field string) error

// submodelElementMetadataCommonFields lists the SubmodelElementAttributes of the
// Part 2 metadata schemas, which every submodel element metadata object shares.
var submodelElementMetadataCommonFields = map[string]struct{}{
	"extensions":                 {},
	"category":                   {},
	"idShort":                    {},
	"displayName":                {},
	"description":                {},
	"modelType":                  {},
	"embeddedDataSpecifications": {},
	"semanticId":                 {},
	"supplementalSemanticIds":    {},
	"qualifiers":                 {},
	"kind":                       {},
}

// submodelElementTypeSpecificMetadataFields lists the fields that the Part 2
// *Metadata schema of a model type adds to the common attributes.
var submodelElementTypeSpecificMetadataFields = map[string]map[string]submodelElementMetadataFieldValidator{
	"Property": {
		"valueType": enumFieldValidator(stringification.DataTypeDefXSDFromString),
	},
	"Range": {
		"valueType": enumFieldValidator(stringification.DataTypeDefXSDFromString),
	},
	"SubmodelElementList": {
		"orderRelevant":         parsedFieldValidator(parseBoolField),
		"semanticIdListElement": parsedFieldValidator(parseReference),
		"typeValueListElement":  enumFieldValidator(stringification.AASSubmodelElementsFromString),
		"valueTypeListElement":  enumFieldValidator(stringification.DataTypeDefXSDFromString),
	},
	"BasicEventElement": {
		"direction":     enumFieldValidator(stringification.DirectionFromString),
		"state":         enumFieldValidator(stringification.StateOfEventFromString),
		"messageTopic":  parsedFieldValidator(parseStringField),
		"messageBroker": parsedFieldValidator(parseReference),
		"lastUpdate":    parsedFieldValidator(parseStringField),
		"minInterval":   parsedFieldValidator(parseStringField),
		"maxInterval":   parsedFieldValidator(parseStringField),
	},
}

// IsSubmodelElementMetadataField reports whether field belongs to the metadata
// representation of modelType as defined by the Part 2 *Metadata schemas.
func IsSubmodelElementMetadataField(modelType string, field string) bool {
	if _, isCommon := submodelElementMetadataCommonFields[field]; isCommon {
		return true
	}
	_, isTypeSpecific := submodelElementTypeSpecificMetadataFields[modelType][field]
	return isTypeSpecific
}

func parsedFieldValidator[T any](parse func(raw any, field string) (T, error)) submodelElementMetadataFieldValidator {
	return func(raw any, field string) error {
		_, err := parse(raw, field)
		return err
	}
}

func enumFieldValidator[T any](fromString func(string) (T, bool)) submodelElementMetadataFieldValidator {
	return func(raw any, field string) error {
		_, err := parseEnumField(raw, field, fromString)
		return err
	}
}
