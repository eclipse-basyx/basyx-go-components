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

package aasenvironment

import (
	"context"
	"log/slog"
	"net/url"
	"strings"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
)

// serializationPackageParts collects the thumbnail and supplementary parts of
// an AASX serialization while its identifiables are streamed.
//
// Parts only hold metadata and a streaming consumer, so the collected slices
// grow with the number of packaged files, not with their content. A nil value
// represents a non-AASX serialization and ignores all additions.
type serializationPackageParts struct {
	service               *SerializationAPIService
	specURI               *url.URL
	thumbnails            []serializationThumbnailPart
	supplementaries       []serializationSupplementaryPart
	seenThumbnailURIs     map[string]struct{}
	seenSupplementaryURIs map[string]struct{}
}

func (s *SerializationAPIService) newSerializationPackageParts(format serializationFormat) (*serializationPackageParts, error) {
	if !format.aasx {
		return nil, nil
	}
	if s.persistence.AASRepository == nil || s.persistence.SubmodelRepository == nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEPARTS-NILREPO AAS and submodel repository backends are required")
	}
	specURI, parseErr := url.Parse(format.specURI)
	if parseErr != nil {
		return nil, common.NewInternalServerError("AASENV-SERIALIZEPARTS-PARSESPECURI " + parseErr.Error())
	}
	return &serializationPackageParts{
		service:               s,
		specURI:               specURI,
		seenThumbnailURIs:     make(map[string]struct{}),
		seenSupplementaryURIs: make(map[string]struct{}),
	}, nil
}

// addThumbnail packages the default thumbnail of the AAS and rewrites its
// defaultThumbnail reference to the deterministic packaged target path.
func (parts *serializationPackageParts) addThumbnail(ctx context.Context, aas aastypes.IAssetAdministrationShell) error {
	if parts == nil || aas == nil {
		return nil
	}
	thumbnailPart, resolvedURI, resolveErr := parts.service.resolveSerializationThumbnailPart(ctx, aas.ID())
	if resolveErr != nil || thumbnailPart == nil {
		return resolveErr
	}
	if _, seen := parts.seenThumbnailURIs[resolvedURI]; seen {
		// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
		slog.WarnContext(ctx, "duplicate thumbnail URI skipped", "error.code", "AASENV-SERIALIZETHUMB-SKIPDUPLICATE", "thumbnail_uri", sanitizeLogValue(resolvedURI), "aas.id", sanitizeLogValue(aas.ID()))
		return nil
	}
	parts.seenThumbnailURIs[resolvedURI] = struct{}{}
	rewriteSerializationThumbnailReference(aas, resolvedURI, thumbnailPart.ContentType)
	parts.thumbnails = append(parts.thumbnails, *thumbnailPart)
	return nil
}

// addSupplementaries packages the internal files referenced by File elements
// of the submodel and rewrites their values to the packaged targets. External,
// unresolved, empty, missing, and duplicate references are skipped.
func (parts *serializationPackageParts) addSupplementaries(ctx context.Context, submodel aastypes.ISubmodel) error {
	if parts == nil {
		return nil
	}
	for _, fileLocation := range CollectSubmodelAASXFileElementLocations(submodel) {
		part, resolvedReference, resolveErr := parts.service.resolveSerializationSupplementaryPart(ctx, parts.specURI, fileLocation)
		if resolveErr != nil {
			return resolveErr
		}
		if part == nil {
			continue
		}
		if _, seen := parts.seenSupplementaryURIs[resolvedReference]; seen {
			// #nosec G706 -- values are escaped by sanitizeLogValue to prevent control-character log injection.
			slog.WarnContext(ctx, "duplicate supplementary URI skipped", "error.code", "AASENV-SERIALIZESUPPL-SKIPDUPLICATE", "supplementary_uri", sanitizeLogValue(resolvedReference), "submodel.id", sanitizeLogValue(fileLocation.SubmodelID), "id_short_path", sanitizeLogValue(fileLocation.IDShortPath))
			continue
		}
		parts.seenSupplementaryURIs[resolvedReference] = struct{}{}
		parts.supplementaries = append(parts.supplementaries, *part)
	}
	return nil
}

// rewriteSerializationThumbnailReference points the AAS defaultThumbnail to the
// packaged thumbnail path. If the asset information has no thumbnail yet, a
// new resource is created.
func rewriteSerializationThumbnailReference(aas aastypes.IAssetAdministrationShell, thumbnailPath, thumbnailContentType string) {
	assetInformation := aas.AssetInformation()
	if assetInformation == nil {
		return
	}

	defaultThumbnail := assetInformation.DefaultThumbnail()
	if defaultThumbnail == nil {
		defaultThumbnail = aastypes.NewResource(thumbnailPath)
		assetInformation.SetDefaultThumbnail(defaultThumbnail)
	} else {
		defaultThumbnail.SetPath(thumbnailPath)
	}

	resolvedThumbnailContentType := strings.TrimSpace(thumbnailContentType)
	if resolvedThumbnailContentType != "" {
		defaultThumbnail.SetContentType(&resolvedThumbnailContentType)
	}
}

// conceptDescriptionReferences collects, in first-seen order, the identifiers
// of concept descriptions referenced by serialized submodels or by included
// concept descriptions. A nil value disables collection.
type conceptDescriptionReferences struct {
	ids  []string
	seen map[string]struct{}
}

func newConceptDescriptionReferences() *conceptDescriptionReferences {
	return &conceptDescriptionReferences{seen: make(map[string]struct{})}
}

// collect records every reference below the instance. This covers semantic
// IDs, list element semantics inherited by list children (AASd-107/AASd-115),
// value IDs, isCaseOf, and IEC 61360 units and value lists.
func (references *conceptDescriptionReferences) collect(instance aastypes.IClass) {
	if references == nil || instance == nil {
		return
	}
	instance.Descend(func(descendant aastypes.IClass) bool {
		if reference, ok := descendant.(aastypes.IReference); ok {
			references.collectReference(reference)
		}
		return false
	})
}

// collectReference records the identifier a reference points to. The first
// key of a model reference names the concept description, while the first key
// of an external reference holds the global identifier that concept
// descriptions conventionally reuse as their id.
func (references *conceptDescriptionReferences) collectReference(reference aastypes.IReference) {
	if len(reference.Keys()) == 0 || reference.Keys()[0] == nil {
		return
	}
	key := reference.Keys()[0]
	if key.Type() != aastypes.KeyTypesConceptDescription && key.Type() != aastypes.KeyTypesGlobalReference {
		return
	}
	id := key.Value()
	if _, seen := references.seen[id]; seen {
		return
	}
	references.seen[id] = struct{}{}
	references.ids = append(references.ids, id)
}
