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

package common

import (
	"net/http"
	"net/url"
	"strings"
)

// ContextualizeAPIResourceLocation resolves a root-relative API Location
// against the request's external origin and mounted context path.
func ContextualizeAPIResourceLocation(request *http.Request, location string, resourcePathMarker string) string {
	target, err := url.Parse(location)
	if err != nil || target.IsAbs() || !strings.HasPrefix(target.Path, resourcePathMarker) {
		return location
	}

	requestPath := request.URL.EscapedPath()
	resourcePathIndex := strings.Index(requestPath, resourcePathMarker)
	if resourcePathIndex < 0 {
		return location
	}

	if externalBaseURL := ExternalBaseURLFromContext(request.Context()); externalBaseURL != "" {
		return locationAgainstExternalBaseURL(externalBaseURL, target)
	}

	contextPath := requestPath[:resourcePathIndex]
	targetPath := target.EscapedPath()
	host := RequestHost(request)
	if host == "" {
		return appendLocationQuery(contextPath+targetPath, target.RawQuery)
	}

	return appendLocationQuery(RequestScheme(request)+"://"+host+contextPath+targetPath, target.RawQuery)
}

func locationAgainstExternalBaseURL(externalBaseURL string, target *url.URL) string {
	externalURL, err := url.Parse(externalBaseURL)
	if err != nil {
		return target.String()
	}

	basePath := strings.TrimSuffix(externalURL.Path, "/")
	baseEscapedPath := strings.TrimSuffix(externalURL.EscapedPath(), "/")
	externalURL.Path = basePath + target.Path
	externalURL.RawPath = baseEscapedPath + target.EscapedPath()
	externalURL.RawQuery = target.RawQuery
	externalURL.Fragment = ""
	return externalURL.String()
}

func appendLocationQuery(location string, rawQuery string) string {
	if rawQuery == "" {
		return location
	}
	return location + "?" + rawQuery
}
