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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package asyncbulk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandAtomicFailures_ExpandsRootFailureToEachItem(t *testing.T) {
	itemIdentifiers := []string{"id-1", "id-2", "id-3"}
	rootFailure := ItemFailure{
		Index:      1,
		Identifier: "id-2",
		StatusCode: 409,
		Message:    "conflict",
	}

	failures := ExpandAtomicFailures(itemIdentifiers, rootFailure)
	require.Len(t, failures, 3)

	require.Equal(t, 1, failures[1].Index)
	require.Equal(t, "id-2", failures[1].Identifier)
	require.Equal(t, 409, failures[1].StatusCode)
	require.Equal(t, "conflict", failures[1].Message)

	require.Equal(t, 0, failures[0].Index)
	require.Equal(t, "id-1", failures[0].Identifier)
	require.Equal(t, 409, failures[0].StatusCode)
	require.Contains(t, failures[0].Message, "rolled back due to atomic failure at index 1")

	require.Equal(t, 2, failures[2].Index)
	require.Equal(t, "id-3", failures[2].Identifier)
	require.Equal(t, 409, failures[2].StatusCode)
	require.Contains(t, failures[2].Message, "rolled back due to atomic failure at index 1")
}

func TestToMessages_CreatesOneMessagePerFailure(t *testing.T) {
	failures := []ItemFailure{
		{Index: 0, Identifier: "id-1", StatusCode: 400, Message: "bad request"},
		{Index: 1, Identifier: "id-2", StatusCode: 409, Message: "conflict"},
	}

	messages := ToMessages(failures)
	require.Len(t, messages, 2)

	require.Equal(t, "400", messages[0].Code)
	require.Equal(t, "Error", messages[0].MessageType)
	require.Contains(t, messages[0].Text, "item[0] (id-1)")
	require.NotEmpty(t, messages[0].Timestamp)
	require.Equal(t, "bulk-item-0", messages[0].CorrelationID)

	require.Equal(t, "409", messages[1].Code)
	require.Equal(t, "Error", messages[1].MessageType)
	require.Contains(t, messages[1].Text, "item[1] (id-2)")
	require.NotEmpty(t, messages[1].Timestamp)
	require.Equal(t, "bulk-item-1", messages[1].CorrelationID)
}
