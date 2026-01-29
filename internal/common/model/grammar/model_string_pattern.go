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
* THE SOFTWARE IS PROVIdED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/

// Package grammar defines the data structures for representing model string patterns in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
//
//nolint:all
package grammar

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// ModelStringPattern represents a string pattern for model references in the AAS grammar.
//
// This type defines valid patterns for referencing elements within Asset Administration Shells,
// Submodels, Submodel Elements, Concept Descriptions, and their descriptors. The pattern must
// match one of the predefined formats starting with prefixes like $aas#, $sm#, $sme#, $cd#,
// $aasdesc#, or $smdesc# followed by specific path expressions.
//
// Valid pattern prefixes:
//   - $aas#      : Asset Administration Shell references
//   - $sm#       : Submodel references
//   - $sme#      : Submodel Element references
//   - $cd#       : Concept Description references
//   - $aasdesc#  : AAS Descriptor references
//   - $smdesc#   : Submodel Descriptor references
//
// Examples:
//   - "$aas#idShort"
//   - "$sm#semanticId.keys[0].value"
//   - "$sme.property1#value"
//   - "$aasdesc#endpoints[0].interface"
type ModelStringPattern string

// UnmarshalJSON implements the json.Unmarshaler interface for ModelStringPattern.
//
// This custom unmarshaler validates that the JSON string value matches the required
// pattern for model string references. The pattern ensures that only valid AAS element
// references are accepted, preventing malformed or invalid path expressions.
//
// Parameters:
//   - value: JSON byte slice containing the string value to unmarshal
//
// Returns:
//   - error: An error if the JSON is invalid or if the string doesn't match the required pattern.
//     The error message includes the pattern that must be matched.
func (j *ModelStringPattern) UnmarshalJSON(value []byte) error {
	type Plain ModelStringPattern
	var plain Plain

	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`, string(plain)); !matched {
		// Fallback for SME idShortPath form like "$sme.temperature#value".
		// This keeps the overall validator strict while unblocking SME queries.
		if strings.HasPrefix(string(plain), "$sme") {
			if smeMatched, _ := regexp.MatchString(`^\$sme(?:\.(?:[a-zA-Z](?:[a-zA-Z0-9_-]*[a-zA-Z0-9_])?(?:\[\d*\])*)*)#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|value|valueType|language)$`, string(plain)); smeMatched {
				*j = ModelStringPattern(plain)
				return nil
			}
		}
		return fmt.Errorf("field %s pattern match: must match %s", "", `^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`)
	}
	*j = ModelStringPattern(plain)
	return nil
}
