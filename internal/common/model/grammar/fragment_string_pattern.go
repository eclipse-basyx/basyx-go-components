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
// Author: Martin Stemmer ( Fraunhofer IESE )

package grammar

import (
	"fmt"
	"regexp"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// FragmentStringPattern represents a string pattern for model references in the AAS grammar.
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
//   - $bd#       : Basic Discovery references
//
// Examples:
//   - "$aas#idShort"
//   - "$sm#semanticId.keys[0]"
//   - "$sme.property1#value"
//   - "$aasdesc#endpoints[0].interface"
//   - "$bd#specificAssetIds[]"
type FragmentStringPattern string

// UnmarshalJSON implements the json.Unmarshaler interface for FragmentStringPattern.
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
func (j *FragmentStringPattern) UnmarshalJSON(value []byte) error {
	type Plain FragmentStringPattern
	var plain Plain

	if err := common.UnmarshalAndDisallowUnknownFields(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^(?:\$aas#(?:idShort|id|assetInformation|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.specificAssetIds\[\d*\](?:\.(?:name|value|externalSubjectId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?))?|submodels|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|(?:\$sm#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id))|(?:\$sme(?:\.(?:[a-zA-Z](?:[a-zA-Z0-9_-]*[a-zA-Z0-9_])?(?:\[\d*\])*)*)*)#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[\d*\](?:\.(?:name|value|externalSubjectId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?))?|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?|submodelDescriptors\[\d*\](?:\.(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?))?))|(?:\$bd#(?:specificAssetIds\[\d*\](?:\.externalSubjectId\.keys\[\d*\])?))|(?:\$smdesc#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?))$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^(?:\$aas#(?:idShort|id|assetInformation|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.specificAssetIds\[\d*\](?:\.(?:name|value|externalSubjectId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?))?|submodels|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|(?:\$sm#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id))|(?:\$sme(?:\.(?:[a-zA-Z](?:[a-zA-Z0-9_-]*[a-zA-Z0-9_])?(?:\[\d*\])*)*)*)#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[\d*\](?:\.(?:name|value|externalSubjectId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?))?|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?|submodelDescriptors\[\d*\](?:\.(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?))?))|(?:\$bd#(?:specificAssetIds\[\d*\](?:\.externalSubjectId\.keys\[\d*\])?))|(?:\$smdesc#(?:semanticId(?:\.(?:type|keys\[\d*\](?:\.(?:type|value))?))?|idShort|id|endpoints\[\d*\](?:\.(?:interface|protocolinformation(?:\.href)?))?))$`)
	}
	*j = FragmentStringPattern(plain)
	return nil
}
