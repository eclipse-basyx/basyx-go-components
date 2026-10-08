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
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// maxBufferedRepresentation bounds the memory used to compute the entity tag
// of a JSON representation. Larger representations are streamed without an
// entity tag.
const maxBufferedRepresentation = 16 << 20

// responseWriter adds entity tags to responses and answers conditional GETs.
// It formats responses only; preconditions of writes are enforced before
// commit.
type responseWriter struct {
	http.ResponseWriter
	state       *State
	requestURI  string
	wroteHeader bool
	status      int
	buffer      *bytes.Buffer
	validator   string
	discard     bool
}

func newResponseWriter(w http.ResponseWriter, state *State, requestURI string) *responseWriter {
	return &responseWriter{ResponseWriter: w, state: state, requestURI: requestURI}
}

func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *responseWriter) Flush() {
	if w.buffer != nil {
		return
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.discard {
		return len(p), nil
	}
	if w.buffer != nil {
		if w.buffer.Len()+len(p) > maxBufferedRepresentation {
			w.streamOversizedRepresentation()
			return w.Write(p)
		}
		return w.buffer.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// streamOversizedRepresentation sends a representation that is too large to
// buffer without an entity tag. A GET with If-Match listing entity tags then
// fails, because the representation's entity tag cannot be determined.
func (w *responseWriter) streamOversizedRepresentation() {
	buffered := w.buffer.Bytes()
	w.buffer = nil
	if ifMatch := w.state.conds.ifMatch; ifMatch.present && !ifMatch.any {
		w.replaceWithPreconditionError(errIfMatchFailed())
		return
	}
	w.ResponseWriter.WriteHeader(http.StatusOK)
	// #nosec G705 -- buffered is the handler's own response, written unchanged.
	_, _ = w.ResponseWriter.Write(buffered)
}

func (w *responseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	if failure := w.state.currentFailure(); failure != nil && code >= http.StatusBadRequest {
		w.replaceWithPreconditionError(failure)
		return
	}
	if isSafeMethod(w.state.method) {
		w.beginRead(code)
		return
	}
	w.beginWrite(code)
}

func (w *responseWriter) beginRead(code int) {
	if code != http.StatusOK || !w.state.observesReads() {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	w.validator = w.state.readValidator()
	if w.validator != "" && isSerializedJSON(w.Header()) {
		w.buffer = &bytes.Buffer{}
		return
	}
	representation := ""
	if w.validator != "" {
		representation = representationTag(w.validator, []byte(w.requestURI))
	}
	w.respondToRead(representation, nil)
}

// respondToRead sends the status of a successful GET after evaluating its
// preconditions. body is the buffered representation, if any.
func (w *responseWriter) respondToRead(representation string, body []byte) {
	switch evaluateRead(w.state.conds, w.validator, representation) {
	case readPreconditionFailed:
		w.replaceWithPreconditionError(errIfMatchFailed())
	case readNotModified:
		w.discard = true
		header := w.Header()
		header.Del("Content-Type")
		header.Del("Content-Length")
		setETag(header, representation)
		w.status = http.StatusNotModified
		w.ResponseWriter.WriteHeader(http.StatusNotModified)
	default:
		setETag(w.Header(), representation)
		w.ResponseWriter.WriteHeader(http.StatusOK)
		if body != nil {
			// #nosec G705 -- body is the handler's own buffered response, written unchanged.
			_, _ = w.ResponseWriter.Write(body)
		}
	}
}

func (w *responseWriter) beginWrite(code int) {
	if code == http.StatusConflict && w.state.mode == ModeResource && w.state.conds.ifNoneMatch.any {
		w.replaceWithPreconditionError(errIfNoneMatchFailed())
		return
	}
	if code >= http.StatusOK && code < http.StatusMultipleChoices && writeReturnsValidator(w.state.method) {
		setETag(w.Header(), unquote(w.state.currentWriteETag()))
	}
	w.ResponseWriter.WriteHeader(code)
}

// writeReturnsValidator reports whether a successful response may carry the
// new entity tag. PUT responses must not (RFC 9110 section 9.3.4), because
// stored representations are normalized.
func writeReturnsValidator(method string) bool {
	return method == http.MethodPatch || method == http.MethodPost
}

func (w *responseWriter) replaceWithPreconditionError(err error) {
	w.discard = true
	w.Header().Del("Content-Length")
	w.Header().Del("ETag")
	w.status = writePreconditionError(w.ResponseWriter, err)
}

// finish completes buffered GET responses and reports conditional writes
// that never evaluated their precondition.
func (w *responseWriter) finish(ctx context.Context) {
	if w.buffer != nil && !w.discard {
		body := w.buffer.Bytes()
		w.buffer = nil
		w.respondToRead(representationTag(w.validator, body), body)
	}
	if w.state.evaluatesWrites() && w.status >= http.StatusOK && w.status < http.StatusMultipleChoices && !w.state.isEvaluated() {
		slog.ErrorContext(ctx, "conditional request changed its target without evaluating the precondition",
			"error.code", "COMMON-CONDREQ-NOTEVALUATED", "method", w.state.method)
	}
}

func writePreconditionError(w http.ResponseWriter, err error) int {
	status := http.StatusPreconditionFailed
	var preconditionErr *PreconditionError
	if errors.As(err, &preconditionErr) {
		status = preconditionErr.HTTPStatus()
	}
	_ = model.WriteErrorResponse(w, err, status, "COMMON", "Conditional", "Precondition")
	return status
}

func setETag(header http.Header, opaque string) {
	if opaque != "" {
		header.Set("ETag", quote(opaque))
	}
}

func unquote(tag string) string {
	return strings.Trim(tag, `"`)
}

// isSerializedJSON reports whether a response is a JSON serialization that
// is buffered to compute its entity tag. File downloads are streamed, even
// when their media type is JSON based, such as application/aasx+json.
func isSerializedJSON(header http.Header) bool {
	if header.Get("Content-Disposition") != "" {
		return false
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(header.Get("Content-Type"), ";")[0]))
	return mediaType == "application/json"
}
