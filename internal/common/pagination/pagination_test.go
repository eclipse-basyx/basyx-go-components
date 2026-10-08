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

package pagination

import (
	"math"
	"testing"
)

func TestMaxSupportedLimitLeavesRoomForLookaheadRow(t *testing.T) {
	if lookahead := int32(MaxSupportedLimit) + 1; lookahead != math.MaxInt32 {
		t.Fatalf("expected lookahead row to fit into int32, got %d", lookahead)
	}
}

func TestBufferCapacity(t *testing.T) {
	cases := map[string]struct {
		limit int
		want  int
	}{
		"negative":       {-5, 0},
		"zero":           {0, 0},
		"small page":     {101, 101},
		"at bound":       {maxBufferCapacity, maxBufferCapacity},
		"above bound":    {maxBufferCapacity + 1, maxBufferCapacity},
		"largest limit":  {MaxSupportedLimit + 1, maxBufferCapacity},
		"beyond integer": {math.MaxInt, maxBufferCapacity},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := BufferCapacity(tc.limit); got != tc.want {
				t.Fatalf("BufferCapacity(%d) = %d, want %d", tc.limit, got, tc.want)
			}
		})
	}
}
