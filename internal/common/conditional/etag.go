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

// Package conditional implements HTTP conditional requests (RFC 9110) for
// BaSyx resources: entity tags, If-Match, If-None-Match and the
// server-managed resource revisions they are based on.
package conditional

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// Kind names a resource kind that carries its own revision.
type Kind string

// Resource kinds with a revision.
const (
	KindAAS                Kind = "aas"
	KindSubmodel           Kind = "submodel"
	KindConceptDescription Kind = "conceptDescription"
	KindAASDescriptor      Kind = "aasDescriptor"
	KindSubmodelDescriptor Kind = "submodelDescriptor"
	KindDiscoveryEntry     Kind = "discoveryEntry"
	KindAASXPackage        Kind = "aasxPackage"
	KindCompanyDescriptor  Kind = "companyDescriptor"
	KindDPP                Kind = "dpp"
)

// ResourceRef identifies a top-level resource by kind and API identifier.
type ResourceRef struct {
	Kind       Kind
	Identifier string
}

// Ref builds a ResourceRef.
func Ref(kind Kind, identifier string) ResourceRef {
	return ResourceRef{Kind: kind, Identifier: identifier}
}

func (r ResourceRef) objectKey() string {
	return string(r.Kind) + ":" + r.Identifier
}

func (r ResourceRef) less(other ResourceRef) bool {
	if r.Kind != other.Kind {
		return r.Kind < other.Kind
	}
	return r.Identifier < other.Identifier
}

// FormatObjectETag returns the strong entity tag of an object revision. The
// tag is bound to the object, so the tag of one object never matches another
// object with the same revision number.
func FormatObjectETag(objectKey string, revision int64) string {
	return quote(objectValidator(objectKey, revision))
}

// ConcurrencyETag returns the entity tag clients use in If-Match for writes.
func ConcurrencyETag(ref ResourceRef, revision int64) string {
	return quote(concurrencyValidator(ref, revision))
}

func concurrencyValidator(ref ResourceRef, revision int64) string {
	return objectValidator(ref.objectKey(), revision)
}

func objectValidator(objectKey string, revision int64) string {
	return strconv.FormatInt(revision, 10) + "-" + digestHex(objectKey, 4)
}

// compositeValidator binds a composite resource to the revisions of all its
// members. The leading number is the highest member revision.
func compositeValidator(ref ResourceRef, members []ResourceRef, revisions map[ResourceRef]int64) string {
	parts := []string{ref.objectKey()}
	highest := int64(0)
	for _, member := range sortedRefs(members) {
		revision := revisions[member]
		highest = max(highest, revision)
		parts = append(parts, member.objectKey()+"="+strconv.FormatInt(revision, 10))
	}
	return strconv.FormatInt(highest, 10) + "-" + digestHex(strings.Join(parts, "|"), 8)
}

// representationTag extends a concurrency validator with a digest of the
// representation data, so different representations or views of the same
// revision never share a tag.
func representationTag(validator string, representation []byte) string {
	digest := sha256.Sum256(representation)
	return validator + "-" + hex.EncodeToString(digest[:8])
}

func digestHex(value string, size int) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:size])
}

func quote(opaque string) string {
	return `"` + opaque + `"`
}

// validatorPrefix returns the "<revision>-<binding>" part of an opaque tag.
func validatorPrefix(opaque string) string {
	first := strings.IndexByte(opaque, '-')
	if first < 0 {
		return opaque
	}
	second := strings.IndexByte(opaque[first+1:], '-')
	if second < 0 {
		return opaque
	}
	return opaque[:first+1+second]
}

type entityTag struct {
	weak   bool
	opaque string
}

// condition is a parsed If-Match or If-None-Match field.
type condition struct {
	present bool
	any     bool
	tags    []entityTag
}

// parseCondition parses all field lines of a conditional header. Malformed
// members end parsing of their field line, so they never match.
func parseCondition(lines []string) condition {
	parsed := condition{present: len(lines) > 0}
	for _, line := range lines {
		parsed.parseLine(line)
	}
	return parsed
}

func (c *condition) parseLine(line string) {
	rest := line
	for {
		rest = strings.TrimLeft(rest, " \t,")
		if rest == "" {
			return
		}
		if rest[0] == '*' {
			c.any = true
			rest = rest[1:]
			continue
		}
		tag, remaining, ok := parseEntityTag(rest)
		if !ok {
			return
		}
		c.tags = append(c.tags, tag)
		rest = remaining
	}
}

func parseEntityTag(value string) (entityTag, string, bool) {
	tag := entityTag{}
	if strings.HasPrefix(value, "W/") {
		tag.weak = true
		value = value[2:]
	}
	if value == "" || value[0] != '"' {
		return tag, "", false
	}
	end := strings.IndexByte(value[1:], '"')
	if end < 0 {
		return tag, "", false
	}
	tag.opaque = value[1 : end+1]
	return tag, value[end+2:], true
}

// matchesStrong reports whether a strong tag designates the validator.
func (c condition) matchesStrong(validator string) bool {
	if validator == "" {
		return false
	}
	for _, tag := range c.tags {
		if !tag.weak && validatorPrefix(tag.opaque) == validator {
			return true
		}
	}
	return false
}

// matchesWeak reports whether any tag designates the validator, ignoring
// weakness.
func (c condition) matchesWeak(validator string) bool {
	if validator == "" {
		return false
	}
	for _, tag := range c.tags {
		if validatorPrefix(tag.opaque) == validator {
			return true
		}
	}
	return false
}

// matchesRepresentationStrong compares complete tags strongly.
func (c condition) matchesRepresentationStrong(opaque string) bool {
	for _, tag := range c.tags {
		if !tag.weak && tag.opaque == opaque {
			return true
		}
	}
	return false
}

// matchesRepresentation compares complete tags weakly.
func (c condition) matchesRepresentation(opaque string) bool {
	for _, tag := range c.tags {
		if tag.opaque == opaque {
			return true
		}
	}
	return false
}
