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
	"fmt"
	"net/url"
	"path"
	"strings"

	aastypes "github.com/aas-core-works/aas-core3.1-golang/types"
)

// AASXFileElementLocation represents a File element and its location in a submodel.
type AASXFileElementLocation struct {
	SubmodelID  string
	IDShortPath string
	FileValue   string
	FileElement *aastypes.File
}

// AASXFileLocation represents a File element and its location in a submodel.
type AASXFileLocation struct {
	SubmodelID  string
	IDShortPath string
	FileValue   string
}

// CollectAASXFileLocations returns all File element locations in the environment.
func CollectAASXFileLocations(environment aastypes.IEnvironment) []AASXFileLocation {
	elementLocations := CollectAASXFileElementLocations(environment)
	locations := make([]AASXFileLocation, 0, len(elementLocations))
	for _, location := range elementLocations {
		locations = append(locations, AASXFileLocation{
			SubmodelID:  location.SubmodelID,
			IDShortPath: location.IDShortPath,
			FileValue:   location.FileValue,
		})
	}

	return locations
}

// CollectAASXFileElementLocations returns all File element locations together
// with mutable File element handles from the environment.
func CollectAASXFileElementLocations(environment aastypes.IEnvironment) []AASXFileElementLocation {
	if environment == nil {
		return nil
	}

	locations := make([]AASXFileElementLocation, 0)
	for _, submodel := range environment.Submodels() {
		if submodel == nil {
			continue
		}
		walkAASXFileElements(submodel.ID(), submodel.SubmodelElements(), "", false, &locations)
	}

	return locations
}

func walkAASXFileElements(
	submodelID string,
	elements []aastypes.ISubmodelElement,
	parentPath string,
	isFromList bool,
	locations *[]AASXFileElementLocation,
) {
	for position, element := range elements {
		if element == nil {
			continue
		}

		idShort := ""
		if element.IDShort() != nil {
			idShort = *element.IDShort()
		}

		idShortPath := buildAASXIDShortPath(parentPath, isFromList, position, idShort)
		if element.ModelType() == aastypes.ModelTypeFile {
			if fileElement, ok := element.(*aastypes.File); ok && fileElement.Value() != nil && strings.TrimSpace(*fileElement.Value()) != "" && idShortPath != "" {
				*locations = append(*locations, AASXFileElementLocation{
					SubmodelID:  submodelID,
					IDShortPath: idShortPath,
					FileValue:   *fileElement.Value(),
					FileElement: fileElement,
				})
			}
		}

		children := extractAASXSubmodelElementChildren(element)
		if len(children) == 0 {
			continue
		}

		walkAASXFileElements(
			submodelID,
			children,
			idShortPath,
			element.ModelType() == aastypes.ModelTypeSubmodelElementList,
			locations,
		)
	}
}

func extractAASXSubmodelElementChildren(element aastypes.ISubmodelElement) []aastypes.ISubmodelElement {
	switch element.ModelType() {
	case aastypes.ModelTypeSubmodelElementCollection:
		if collection, ok := element.(*aastypes.SubmodelElementCollection); ok {
			return collection.Value()
		}
	case aastypes.ModelTypeSubmodelElementList:
		if list, ok := element.(*aastypes.SubmodelElementList); ok {
			return list.Value()
		}
	case aastypes.ModelTypeAnnotatedRelationshipElement:
		if annotated, ok := element.(*aastypes.AnnotatedRelationshipElement); ok {
			children := make([]aastypes.ISubmodelElement, 0, len(annotated.Annotations()))
			for _, annotation := range annotated.Annotations() {
				children = append(children, annotation)
			}
			return children
		}
	case aastypes.ModelTypeEntity:
		if entity, ok := element.(*aastypes.Entity); ok {
			return entity.Statements()
		}
	}

	return nil
}

func buildAASXIDShortPath(parentPath string, isFromList bool, position int, idShort string) string {
	if parentPath == "" {
		if isFromList {
			return "[" + fmt.Sprintf("%d", position) + "]"
		}
		if strings.TrimSpace(idShort) == "" {
			return ""
		}
		return idShort
	}
	if isFromList {
		return parentPath + "[" + fmt.Sprintf("%d", position) + "]"
	}
	if strings.TrimSpace(idShort) == "" {
		return parentPath
	}
	return parentPath + "." + idShort
}

// IsExternalAASXReference returns true for http/https file references.
func IsExternalAASXReference(reference string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(reference))
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

// MatchAASXSupplementaryTarget checks whether a file value resolves to a supplementary URI.
func MatchAASXSupplementaryTarget(fileValue string, specURI *url.URL, supplementaryURI *url.URL) bool {
	reference := strings.TrimSpace(fileValue)
	if reference == "" {
		return false
	}
	if IsExternalAASXReference(reference) {
		return false
	}

	resolvedReference := ResolveAASXReferenceAgainstSpec(reference, specURI)
	resolvedSupplementary := NormalizeAASXPartURI(supplementaryURI)
	return resolvedReference != "" && resolvedReference == resolvedSupplementary
}

// ResolveAASXReferenceAgainstSpec resolves a file reference relative to the AASX spec URI.
func ResolveAASXReferenceAgainstSpec(reference string, specURI *url.URL) string {
	referenceURL, err := url.Parse(reference)
	if err != nil {
		return ""
	}
	if referenceURL.IsAbs() {
		return NormalizeAASXPartURI(referenceURL)
	}

	if specURI == nil {
		if strings.HasPrefix(reference, "/") {
			parsed, parseErr := url.Parse(reference)
			if parseErr != nil {
				return ""
			}
			return NormalizeAASXPartURI(parsed)
		}
		return ""
	}

	base := &url.URL{Path: NormalizeAASXPartURI(specURI)}
	return NormalizeAASXPartURI(base.ResolveReference(referenceURL))
}

// ResolveAASXSerializationSupplementaryPath maps a file reference to the
// serialization target under supplementaryRootPath while preserving relative subfolders.
func ResolveAASXSerializationSupplementaryPath(reference string, specURI *url.URL, supplementaryRootPath string) string {
	resolvedReference := ResolveAASXReferenceAgainstSpec(reference, specURI)
	if resolvedReference == "" {
		return ""
	}

	normalizedSupplementaryRootPath := NormalizeAASXPartURI(&url.URL{Path: supplementaryRootPath})
	if normalizedSupplementaryRootPath == "" || normalizedSupplementaryRootPath == "/" {
		return ""
	}

	if resolvedReference == normalizedSupplementaryRootPath || strings.HasPrefix(resolvedReference, normalizedSupplementaryRootPath+"/") {
		return resolvedReference
	}

	relativePath := strings.TrimPrefix(resolvedReference, "/")
	if specURI != nil {
		specDir := path.Dir(NormalizeAASXPartURI(specURI))
		if specDir != "" && specDir != "/" && specDir != "." {
			specDirPrefix := strings.TrimSuffix(specDir, "/") + "/"
			if strings.HasPrefix(resolvedReference, specDirPrefix) {
				relativePath = strings.TrimPrefix(resolvedReference, specDirPrefix)
			}
		}
	}

	normalizedRelativePath := strings.TrimPrefix(path.Clean("/"+relativePath), "/")
	if normalizedRelativePath == "" || normalizedRelativePath == "." {
		return ""
	}

	resolvedPath := path.Join(normalizedSupplementaryRootPath, normalizedRelativePath)
	if !strings.HasPrefix(resolvedPath, normalizedSupplementaryRootPath+"/") {
		return ""
	}

	return resolvedPath
}

// NormalizeAASXPartURI normalizes package-part URIs to slash-prefixed clean paths.
func NormalizeAASXPartURI(uri *url.URL) string {
	if uri == nil {
		return ""
	}

	uriPath := strings.TrimSpace(uri.Path)
	if uriPath == "" {
		uriPath = strings.TrimSpace(uri.String())
	}
	if uriPath == "" {
		return ""
	}

	uriPath = strings.ReplaceAll(uriPath, "\\", "/")
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	return path.Clean(uriPath)
}
