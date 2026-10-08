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

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConditionHandlesListsWildcardsWeakTagsAndMalformedMembers(t *testing.T) {
	parsed := parseCondition([]string{`"1-a", W/"2-b"`, ` * ,"3-c,d"`, `"4-e" garbage "5-f"`})

	require.True(t, parsed.present)
	require.True(t, parsed.any)
	require.Equal(t, []entityTag{
		{opaque: "1-a"},
		{weak: true, opaque: "2-b"},
		{opaque: "3-c,d"},
		{opaque: "4-e"},
	}, parsed.tags)
}

func TestParseConditionWithoutFieldIsAbsent(t *testing.T) {
	require.False(t, parseCondition(nil).present)
}

func TestConcurrencyETagIsBoundToTheResource(t *testing.T) {
	first := ConcurrencyETag(Ref(KindSubmodel, "urn:a"), 7)
	other := ConcurrencyETag(Ref(KindSubmodel, "urn:b"), 7)
	otherKind := ConcurrencyETag(Ref(KindAAS, "urn:a"), 7)

	require.Regexp(t, `^"7-[0-9a-f]{8}"$`, first)
	require.NotEqual(t, first, other)
	require.NotEqual(t, first, otherKind)
}

func TestFormatObjectETagKeepsTheReBACFormat(t *testing.T) {
	require.Equal(t, `"3-`+digestHex("aas:1234", 4)+`"`, FormatObjectETag("aas:1234", 3))
}

func TestStrongMatchComparesTheConcurrencyPrefixAndRejectsWeakTags(t *testing.T) {
	validator := concurrencyValidator(Ref(KindSubmodel, "urn:a"), 5)
	representation := representationTag(validator, []byte(`{"id":"urn:a"}`))

	require.True(t, parseCondition([]string{quote(representation)}).matchesStrong(validator))
	require.True(t, parseCondition([]string{quote(validator)}).matchesStrong(validator))
	require.False(t, parseCondition([]string{"W/" + quote(validator)}).matchesStrong(validator))
	require.True(t, parseCondition([]string{"W/" + quote(validator)}).matchesWeak(validator))
	require.False(t, parseCondition([]string{quote(concurrencyValidator(Ref(KindSubmodel, "urn:a"), 6))}).matchesStrong(validator))
}

func TestRepresentationTagDiffersPerRepresentation(t *testing.T) {
	validator := concurrencyValidator(Ref(KindSubmodel, "urn:a"), 5)

	require.NotEqual(t, representationTag(validator, []byte("a")), representationTag(validator, []byte("b")))
	require.Equal(t, validator, validatorPrefix(representationTag(validator, []byte("a"))))
}

func TestCompositeValidatorChangesWithAnyMemberRevision(t *testing.T) {
	dpp := Ref(KindDPP, "dpp-1")
	members := []ResourceRef{Ref(KindAAS, "aas-1"), Ref(KindSubmodel, "sm-1")}
	base := compositeValidator(dpp, members, map[ResourceRef]int64{members[0]: 3, members[1]: 9})
	changed := compositeValidator(dpp, members, map[ResourceRef]int64{members[0]: 3, members[1]: 10})
	reordered := compositeValidator(dpp, []ResourceRef{members[1], members[0]}, map[ResourceRef]int64{members[0]: 3, members[1]: 9})

	require.Regexp(t, `^9-[0-9a-f]{16}$`, base)
	require.NotEqual(t, base, changed)
	require.Equal(t, base, reordered)
}

func TestEvaluateWriteFollowsRFC9110Order(t *testing.T) {
	ref := Ref(KindSubmodel, "urn:a")
	current := writeTarget{existed: true, validator: concurrencyValidator(ref, 4)}
	stale := quote(concurrencyValidator(ref, 3))
	fresh := quote(current.validator)
	conds := func(ifMatch, ifNoneMatch []string) conditions {
		return conditions{ifMatch: parseCondition(ifMatch), ifNoneMatch: parseCondition(ifNoneMatch)}
	}

	require.NoError(t, evaluateWrite(conds(nil, nil), current, false))
	require.NoError(t, evaluateWrite(conds([]string{fresh}, nil), current, false))
	require.NoError(t, evaluateWrite(conds([]string{"*"}, nil), current, false))
	require.True(t, IsPreconditionFailed(evaluateWrite(conds([]string{stale}, nil), current, false)))
	require.True(t, IsPreconditionFailed(evaluateWrite(conds([]string{"*"}, nil), writeTarget{}, false)))
	require.True(t, IsPreconditionFailed(evaluateWrite(conds(nil, []string{"*"}), current, false)))
	require.NoError(t, evaluateWrite(conds(nil, []string{"*"}), writeTarget{}, false))
	require.True(t, IsPreconditionFailed(evaluateWrite(conds(nil, []string{"W/" + fresh}), current, false)))
	require.NoError(t, evaluateWrite(conds(nil, []string{stale}), current, false))
	require.True(t, IsPreconditionRequired(evaluateWrite(conds(nil, nil), current, true)))
	require.NoError(t, evaluateWrite(conds(nil, nil), writeTarget{}, true))
	require.NoError(t, evaluateWrite(conds(nil, []string{"*"}), writeTarget{}, true))
}

func TestEvaluateReadAnswersPreconditionFailedOrNotModified(t *testing.T) {
	validator := concurrencyValidator(Ref(KindSubmodel, "urn:a"), 4)
	representation := representationTag(validator, []byte("body"))
	conds := parseConditions

	require.Equal(t, readProceed, evaluateRead(conds(http.Header{}), validator, representation))
	require.Equal(t, readNotModified, evaluateRead(conds(http.Header{"If-None-Match": {"W/" + quote(representation)}}), validator, representation))
	require.Equal(t, readProceed, evaluateRead(conds(http.Header{"If-None-Match": {quote(validator)}}), validator, representation))
	require.Equal(t, readPreconditionFailed, evaluateRead(conds(http.Header{"If-Match": {`"1-00000000"`}}), validator, representation))
	require.Equal(t, readProceed, evaluateRead(conds(http.Header{"If-Match": {quote(representation)}}), validator, representation))
	require.Equal(t, readPreconditionFailed, evaluateRead(conds(http.Header{"If-Match": {"*"}}), "", ""))
}
