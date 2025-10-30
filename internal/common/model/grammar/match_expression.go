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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package grammar

import (
	"encoding/json"
	"fmt"
)

type MatchExpression struct {
	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// Contains corresponds to the JSON schema field "$contains".
	Contains StringItems `json:"$contains,omitempty" yaml:"$contains,omitempty" mapstructure:"$contains,omitempty"`

	// EndsWith corresponds to the JSON schema field "$ends-with".
	EndsWith StringItems `json:"$ends-with,omitempty" yaml:"$ends-with,omitempty" mapstructure:"$ends-with,omitempty"`

	// Eq corresponds to the JSON schema field "$eq".
	Eq ComparisonItems `json:"$eq,omitempty" yaml:"$eq,omitempty" mapstructure:"$eq,omitempty"`

	// Ge corresponds to the JSON schema field "$ge".
	Ge ComparisonItems `json:"$ge,omitempty" yaml:"$ge,omitempty" mapstructure:"$ge,omitempty"`

	// Gt corresponds to the JSON schema field "$gt".
	Gt ComparisonItems `json:"$gt,omitempty" yaml:"$gt,omitempty" mapstructure:"$gt,omitempty"`

	// Le corresponds to the JSON schema field "$le".
	Le ComparisonItems `json:"$le,omitempty" yaml:"$le,omitempty" mapstructure:"$le,omitempty"`

	// Lt corresponds to the JSON schema field "$lt".
	Lt ComparisonItems `json:"$lt,omitempty" yaml:"$lt,omitempty" mapstructure:"$lt,omitempty"`

	// Match corresponds to the JSON schema field "$match".
	Match []MatchExpression `json:"$match,omitempty" yaml:"$match,omitempty" mapstructure:"$match,omitempty"`

	// Ne corresponds to the JSON schema field "$ne".
	Ne ComparisonItems `json:"$ne,omitempty" yaml:"$ne,omitempty" mapstructure:"$ne,omitempty"`

	// Regex corresponds to the JSON schema field "$regex".
	Regex StringItems `json:"$regex,omitempty" yaml:"$regex,omitempty" mapstructure:"$regex,omitempty"`

	// StartsWith corresponds to the JSON schema field "$starts-with".
	StartsWith StringItems `json:"$starts-with,omitempty" yaml:"$starts-with,omitempty" mapstructure:"$starts-with,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *MatchExpression) UnmarshalJSON(value []byte) error {
	type Plain MatchExpression
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if plain.Match != nil && len(plain.Match) < 1 {
		return fmt.Errorf("field %s length: must be >= %d", "$match", 1)
	}
	*j = MatchExpression(plain)
	return nil
}
