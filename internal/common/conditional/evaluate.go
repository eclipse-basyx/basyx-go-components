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

package conditional

import "net/http"

// conditions are the conditional header fields of one request.
type conditions struct {
	ifMatch     condition
	ifNoneMatch condition
}

func parseConditions(header http.Header) conditions {
	return conditions{
		ifMatch:     parseCondition(header.Values("If-Match")),
		ifNoneMatch: parseCondition(header.Values("If-None-Match")),
	}
}

func (c conditions) present() bool {
	return c.ifMatch.present || c.ifNoneMatch.present
}

// writeTarget is the state of a write target when its precondition is evaluated.
type writeTarget struct {
	existed   bool
	validator string
}

// evaluateWrite evaluates the preconditions of a state-changing request in
// the order of RFC 9110 section 13.2.2.
func evaluateWrite(conds conditions, target writeTarget, requireIfMatch bool) error {
	if conds.ifMatch.present && !ifMatchHolds(conds.ifMatch, target) {
		return errIfMatchFailed()
	}
	if conds.ifNoneMatch.present && ifNoneMatchMatches(conds.ifNoneMatch, target) {
		return errIfNoneMatchFailed()
	}
	if requireIfMatch && target.existed && !conds.ifMatch.present && !conds.ifNoneMatch.any {
		return errIfMatchRequired()
	}
	return nil
}

func ifMatchHolds(ifMatch condition, target writeTarget) bool {
	if !target.existed {
		return false
	}
	return ifMatch.any || ifMatch.matchesStrong(target.validator)
}

func ifNoneMatchMatches(ifNoneMatch condition, target writeTarget) bool {
	if !target.existed {
		return false
	}
	return ifNoneMatch.any || ifNoneMatch.matchesWeak(target.validator)
}

// readOutcome is the result of evaluating the preconditions of a GET.
type readOutcome int

const (
	readProceed readOutcome = iota
	readPreconditionFailed
	readNotModified
)

// evaluateRead evaluates If-Match and If-None-Match of a GET against the
// concurrency validator and the complete representation tag. An empty
// validator means the server could not determine a tag.
func evaluateRead(conds conditions, validator string, representation string) readOutcome {
	if conds.ifMatch.present && !ifMatchHoldsForRead(conds.ifMatch, validator) {
		return readPreconditionFailed
	}
	if !conds.ifNoneMatch.present {
		return readProceed
	}
	if conds.ifNoneMatch.any || (representation != "" && conds.ifNoneMatch.matchesRepresentation(representation)) {
		return readNotModified
	}
	return readProceed
}

func ifMatchHoldsForRead(ifMatch condition, validator string) bool {
	return validator != "" && (ifMatch.any || ifMatch.matchesStrong(validator))
}
