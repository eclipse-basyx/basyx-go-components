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

// Package rebac provides a bounded, fail-closed client for OpenFGA.
package rebac

import "time"

// Config identifies the OpenFGA store and authorization model used by a client.
type Config struct {
	URL     string
	StoreID string
	ModelID string
	Token   string
	Timeout time.Duration
}

// Tuple identifies one OpenFGA relationship tuple.
type Tuple struct {
	User     string `json:"user"`
	Relation string `json:"relation"`
	Object   string `json:"object"`
}

// BatchCheckRequest identifies one independently correlated authorization check.
type BatchCheckRequest struct {
	CorrelationID    string
	Tuple            Tuple
	ContextualTuples []Tuple
}

// BatchCheckResult contains the decision associated with CorrelationID.
type BatchCheckResult struct {
	CorrelationID string
	Allowed       bool
}

// Change is an OpenFGA tuple change returned by ReadChanges.
type Change struct {
	Tuple     Tuple
	Operation string
	Timestamp time.Time
}

// ChangesPage is one cursor-addressable OpenFGA change page.
type ChangesPage struct {
	Changes           []Change
	ContinuationToken string
}
