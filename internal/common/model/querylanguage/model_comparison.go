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

// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )
package querylanguage

import (
	"encoding/json"
	"fmt"
)

type Comparison struct {
	Eq *Operation `json:"$eq,omitempty"`
	Ne *Operation `json:"$ne,omitempty"`
	Gt *Operation `json:"$gt,omitempty"`
	Ge *Operation `json:"$ge,omitempty"`
	Lt *Operation `json:"$lt,omitempty"`
	Le *Operation `json:"$le,omitempty"`
}

func (c *Comparison) GetOperationType() string {
	if c.Eq != nil {
		return "$eq"
	}
	if c.Ne != nil {
		return "$ne"
	}
	if c.Gt != nil {
		return "$gt"
	}
	if c.Ge != nil {
		return "$ge"
	}
	if c.Lt != nil {
		return "$lt"
	}
	if c.Le != nil {
		return "$le"
	}
	return ""
}

func (c *Comparison) GetOperation() *Operation {
	if c.Eq != nil {
		return c.Eq
	}
	if c.Ne != nil {
		return c.Ne
	}
	if c.Gt != nil {
		return c.Gt
	}
	if c.Ge != nil {
		return c.Ge
	}
	if c.Lt != nil {
		return c.Lt
	}
	if c.Le != nil {
		return c.Le
	}
	return nil
}

func (c *Comparison) IsCondition() bool {
	return true
}

func (c *Comparison) GetConditionType() string {
	return "Comparison"
}

// UnmarshalJSON implements custom unmarshalling for Comparison
func (c *Comparison) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal into a map to see which comparison operator we have
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Handle each comparison operator
	for op, value := range raw {
		switch op {
		case "$eq":
			c.Eq = &Operation{}
			if err := c.unmarshalOperation(value, c.Eq); err != nil {
				return fmt.Errorf("error unmarshalling $eq: %w", err)
			}
		case "$ne":
			c.Ne = &Operation{}
			if err := c.unmarshalOperation(value, c.Ne); err != nil {
				return fmt.Errorf("error unmarshalling $ne: %w", err)
			}
		case "$gt":
			c.Gt = &Operation{}
			if err := c.unmarshalOperation(value, c.Gt); err != nil {
				return fmt.Errorf("error unmarshalling $gt: %w", err)
			}
		case "$ge":
			c.Ge = &Operation{}
			if err := c.unmarshalOperation(value, c.Ge); err != nil {
				return fmt.Errorf("error unmarshalling $ge: %w", err)
			}
		case "$lt":
			c.Lt = &Operation{}
			if err := c.unmarshalOperation(value, c.Lt); err != nil {
				return fmt.Errorf("error unmarshalling $lt: %w", err)
			}
		case "$le":
			c.Le = &Operation{}
			if err := c.unmarshalOperation(value, c.Le); err != nil {
				return fmt.Errorf("error unmarshalling $le: %w", err)
			}
		default:
			return fmt.Errorf("unknown comparison operator: %s", op)
		}
	}

	return nil
}

// unmarshalOperation handles the conversion of JSON to Operation
func (c *Comparison) unmarshalOperation(data []byte, op *Operation) error {
	if op == nil {
		return fmt.Errorf("operation pointer is nil")
	}
	if len(data) == 0 {
		return fmt.Errorf("no data provided for unmarshalling")
	}

	// First, try to unmarshal as an array of operand objects (new structure)
	var operandArray []json.RawMessage
	if err := json.Unmarshal(data, &operandArray); err == nil {
		// New array-based structure
		if len(operandArray) != 2 {
			return fmt.Errorf("exactly 2 operands are required per operation, got %d", len(operandArray))
		}

		var operands []Operand
		for i, operandData := range operandArray {
			var operand Operand
			if err := json.Unmarshal(operandData, &operand); err != nil {
				return fmt.Errorf("failed to unmarshal operand %d: %w", i, err)
			}
			operands = append(operands, operand)
		}

		op.Operands = operands
		return nil
	}

	// Fall back to old flat structure for backward compatibility
	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return fmt.Errorf("failed to unmarshal operation data as both array and object: %w", err)
	}

	var operands []Operand

	// Check each field and create a separate operand for each non-empty field
	if field, exists := rawMap["$field"]; exists && field != nil {
		operands = append(operands, Operand{Field: field.(string)})
	}
	if strVal, exists := rawMap["$strVal"]; exists && strVal != nil {
		operands = append(operands, Operand{StrVal: strVal.(string)})
	}
	if numVal, exists := rawMap["$numVal"]; exists && numVal != nil {
		operands = append(operands, Operand{NumVal: NewNumVal(numVal.(int64))})
	}
	if hexVal, exists := rawMap["$hexVal"]; exists && hexVal != nil {
		operands = append(operands, Operand{HexVal: hexVal.(string)})
	}
	if dateTimeVal, exists := rawMap["$dateTimeVal"]; exists && dateTimeVal != nil {
		operands = append(operands, Operand{DateTimeVal: dateTimeVal.(string)})
	}
	if timeVal, exists := rawMap["$timeVal"]; exists && timeVal != nil {
		operands = append(operands, Operand{TimeVal: timeVal.(string)})
	}
	if dayOfWeek, exists := rawMap["$dayOfWeek"]; exists && dayOfWeek != nil {
		operands = append(operands, Operand{DayOfWeek: dayOfWeek.(string)})
	}
	if dayOfMonth, exists := rawMap["$dayOfMonth"]; exists && dayOfMonth != nil {
		operands = append(operands, Operand{DayOfMonth: dayOfMonth.(string)})
	}
	if month, exists := rawMap["$month"]; exists && month != nil {
		operands = append(operands, Operand{Month: month.(string)})
	}
	if year, exists := rawMap["$year"]; exists && year != nil {
		operands = append(operands, Operand{Year: year.(string)})
	}
	if boolean, exists := rawMap["$boolean"]; exists && boolean != nil {
		boolVal := boolean.(bool)
		if boolVal {
			operands = append(operands, Operand{Boolean: "true"})
		} else {
			operands = append(operands, Operand{Boolean: "false"})
		}
	}

	// If we found operands from the individual fields, use them
	if len(operands) > 0 {
		if len(operands) != 2 {
			return fmt.Errorf("exactly 2 operands are required per operation, got %d", len(operands))
		}
		op.Operands = operands
		return nil
	}

	// Otherwise, try to unmarshal as an Operation with Operands array
	var operation Operation
	if err := json.Unmarshal(data, &operation); err != nil {
		return fmt.Errorf("failed to unmarshal as operation with operands array: %w", err)
	}

	*op = operation
	return nil
}
