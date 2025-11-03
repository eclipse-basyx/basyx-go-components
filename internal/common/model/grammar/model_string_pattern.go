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

// Package grammar defines the data structures for representing model string patterns in the grammar model.
// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
	"regexp"
)

type ModelStringPattern string

// UnmarshalJSON implements json.Unmarshaler.
func (j *ModelStringPattern) UnmarshalJSON(value []byte) error {
	type Plain ModelStringPattern
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectID(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectID(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticID(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`)
	}
	*j = ModelStringPattern(plain)
	return nil
}
