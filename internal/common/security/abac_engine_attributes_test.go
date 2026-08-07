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

package auth

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestAttributesSatisfiedAll(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		items  []grammar.AttributeItem
		claims Claims
		want   bool
	}{
		{
			name: "empty attributes deny access",
			want: false,
		},
		{
			name: "all claims are present",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRCLAIM, Value: "role"},
				{Kind: grammar.ATTRCLAIM, Value: "clearance"},
			},
			claims: Claims{"role": "reader", "clearance": 3},
			want:   true,
		},
		{
			name: "one claim is missing",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRCLAIM, Value: "role"},
				{Kind: grammar.ATTRCLAIM, Value: "clearance"},
			},
			claims: Claims{"role": "reader"},
			want:   false,
		},
		{
			name: "anonymous permits access without claims",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRGLOBAL, Value: "ANONYMOUS"},
			},
			want: true,
		},
		{
			name: "anonymous does not replace missing claims",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRCLAIM, Value: "role"},
				{Kind: grammar.ATTRGLOBAL, Value: "ANONYMOUS"},
			},
			want: false,
		},
		{
			name: "anonymous remains optional when all claims are present",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRCLAIM, Value: "role"},
				{Kind: grammar.ATTRGLOBAL, Value: "ANONYMOUS"},
			},
			claims: Claims{"role": "reader"},
			want:   true,
		},
		{
			name: "date-time globals are optional",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRGLOBAL, Value: "UTCNOW"},
				{Kind: grammar.ATTRGLOBAL, Value: "LOCALNOW"},
				{Kind: grammar.ATTRGLOBAL, Value: "CLIENTNOW"},
				{Kind: grammar.ATTRCLAIM, Value: "role"},
			},
			claims: Claims{"role": "reader"},
			want:   true,
		},
		{
			name: "date-time-only attributes deny access",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRGLOBAL, Value: "UTCNOW"},
				{Kind: grammar.ATTRGLOBAL, Value: "LOCALNOW"},
				{Kind: grammar.ATTRGLOBAL, Value: "CLIENTNOW"},
			},
			want: false,
		},
		{
			name: "date-time globals do not replace missing claims",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRGLOBAL, Value: "UTCNOW"},
				{Kind: grammar.ATTRCLAIM, Value: "role"},
			},
			want: false,
		},
		{
			name: "unknown attribute kind fails closed",
			items: []grammar.AttributeItem{
				{Kind: grammar.ATTRTYPE("UNKNOWN"), Value: "value"},
				{Kind: grammar.ATTRGLOBAL, Value: "ANONYMOUS"},
			},
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := attributesSatisfiedAll(test.items, test.claims); got != test.want {
				t.Fatalf("attributesSatisfiedAll() = %t, want %t", got, test.want)
			}
		})
	}
}
