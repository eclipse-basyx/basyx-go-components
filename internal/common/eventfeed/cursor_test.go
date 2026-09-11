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

package eventfeed

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	enc, err := encodeCursor(42)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := decodeCursor(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.AfterSeq != 42 {
		t.Fatalf("seq=%d", dec.AfterSeq)
	}
}

func TestCursorInvalid(t *testing.T) {
	if _, err := decodeCursor("%%%"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCursorRestoresQueryContext(t *testing.T) {
	since := time.Now().UTC().Add(-time.Hour)
	original := FeedQuery{Since: &since, Filter: "rsql:event.subject==one", Presentation: PresentationCompact, Limit: 2}
	cursor, err := encodeQueryCursor(42, original)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := resolveCursorQuery(FeedQuery{Cursor: cursor, Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if restored.Since == nil || !restored.Since.Equal(since) || restored.Filter != original.Filter || restored.Presentation != PresentationCompact || restored.Limit != 3 {
		t.Fatalf("restored query: %+v", restored)
	}
	for _, request := range []FeedQuery{
		{Cursor: cursor, Filter: "rsql:event.subject==other"},
		{Cursor: cursor, Presentation: PresentationRegular},
		{Cursor: cursor, Since: new(since.Add(-time.Hour))},
		{Cursor: cursor, LastEventID: "another-event"},
	} {
		if _, err := resolveCursorQuery(request); !IsQueryError(err) {
			t.Fatalf("query mismatch accepted: %+v; err=%v", request, err)
		}
	}
}
