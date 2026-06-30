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

package grammar

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelStringPatternUnmarshalAcceptsSupplementalSemanticIds(t *testing.T) {
	t.Parallel()

	cases := []string{
		"$sm#supplementalSemanticIds[].keys[0].value",
		"$sme.machine-state#supplementalSemanticIds[1].keys[].type",
		"$aasdesc#submodelDescriptors[].supplementalSemanticIds[0].keys[2].value",
		"$smdesc#supplementalSemanticIds[].type",
	}

	for _, in := range cases {
		in := in
		t.Run(in, func(t *testing.T) {
			var p ModelStringPattern
			if err := json.Unmarshal([]byte(`"`+in+`"`), &p); err != nil {
				t.Fatalf("expected %q to be valid: %v", in, err)
			}
		})
	}
}

func TestModelStringPatternUnmarshalRejectsLeadingZeroIndex(t *testing.T) {
	t.Parallel()

	var p ModelStringPattern
	err := json.Unmarshal([]byte(`"$sm#supplementalSemanticIds[01].keys[0].value"`), &p)
	if err == nil {
		t.Fatal("expected leading-zero array index to be rejected")
	}
}

func TestAttributeItemUnmarshalAcceptsReferenceIdentifier(t *testing.T) {
	t.Parallel()

	var attr AttributeItem
	if err := json.Unmarshal([]byte(`{"REFERENCE":"$sme(\"SubmodelID-OperationalData\").machineState#value"}`), &attr); err != nil {
		t.Fatalf("expected REFERENCE identifier to be valid: %v", err)
	}
	if attr.Kind != ATTRREFERENCE {
		t.Fatalf("expected REFERENCE kind, got %s", attr.Kind)
	}
}

func TestLogicalExpressionUnmarshalPR88OperatorRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   string
		wantError string
	}{
		{
			name:    "bool cast logical operand",
			payload: `{"$boolCast":{"$strVal":"true"}}`,
		},
		{
			name:      "one operator only",
			payload:   `{"$boolean":true,"$eq":[{"$numVal":1},{"$numVal":1}]}`,
			wantError: "ONEOF",
		},
		{
			name:      "ordered bool comparison rejected",
			payload:   `{"$gt":[{"$boolean":true},{"$boolean":false}]}`,
			wantError: "boolean",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var expr LogicalExpression
			err := json.Unmarshal([]byte(tt.payload), &expr)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestValueUnmarshalPR88OperandRules(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		payload   string
		wantError string
	}{
		{
			name:    "date part uses dateTimeOperand",
			payload: `{"$year":{"$dateTimeVal":"2025-01-01T00:00:00Z"}}`,
		},
		{
			name:      "date part rejects direct literal",
			payload:   `{"$year":"2025-01-01T00:00:00Z"}`,
			wantError: "cannot unmarshal string",
		},
		{
			name:    "dateTimeCast uses stringValue",
			payload: `{"$dateTimeCast":{"$field":"$aasdesc#createdAt"}}`,
		},
		{
			name:      "dateTimeCast rejects numeric operand",
			payload:   `{"$dateTimeCast":{"$numVal":1}}`,
			wantError: "$dateTimeCast requires a stringValue operand",
		},
		{
			name:    "timeCast uses dateTimeOperand",
			payload: `{"$timeCast":{"$dateTimeVal":"2025-01-01T00:00:00Z"}}`,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			var value Value
			err := json.Unmarshal([]byte(tt.payload), &value)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected error containing %q, got %v", tt.wantError, err)
			}
		})
	}
}

func TestMatchExpressionUnmarshalRejectsBooleanOperator(t *testing.T) {
	t.Parallel()

	var expr MatchExpression
	err := json.Unmarshal([]byte(`{"$boolean":true}`), &expr)
	if err == nil {
		t.Fatal("expected $boolean to be rejected for matchExpression")
	}
}
