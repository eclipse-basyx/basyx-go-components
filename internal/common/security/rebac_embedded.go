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

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
	"net/http"
	"net/url"
	"strings"
)

func (s *rebacSecurity) validateEmbeddedMutation(ctx context.Context, tx *sql.Tx, request *rebacRequest, aasID string, deleted bool) error {
	parent, err := s.runtime.State.Resource(ctx, tx, "aas_descriptor", aasID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	children, err := s.runtime.State.EmbeddedChildren(ctx, tx, parent)
	if err != nil {
		return err
	}
	current, err := rebac.EmbeddedDescriptorIDs(ctx, tx, aasID)
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, id := range current {
		live[id] = true
	}
	for _, child := range children {
		if err := s.validateEmbeddedChildMutation(ctx, tx, request, aasID, child, live, deleted); err != nil {
			return err
		}
	}
	return nil
}

func (s *rebacSecurity) authorizeEmbeddedMutation(ctx context.Context, tx *sql.Tx, original *rebacRequest, aasID, submodelID, method string) error {
	path := "/shell-descriptors/" + common.EncodeString(aasID) + "/submodel-descriptors/" + common.EncodeString(submodelID)
	route, err := ClassifyReBACRoute(method, path)
	if err != nil {
		return err
	}
	request := *original
	request.route = route
	ctx = context.WithValue(ctx, rebacRequestKey{}, &request)
	ctx = context.WithValue(ctx, rebacWriterTransactionKey{}, tx)
	path = strings.TrimRight(common.NormalizeBasePath(s.cfg.Server.ContextPath), "/") + path
	r := (&http.Request{Method: method, URL: &url.URL{Path: path}}).WithContext(ctx)
	authorized, err := s.authorizeRequest(r, tx, &request)
	if err != nil {
		return err
	}
	if filter, _ := authorized.Context().Value(filterKey).(*QueryFilter); filter != nil {
		return rebac.ErrForbidden
	}
	return nil
}

// ContextWithReBACEmbeddedRead isolates embedded descriptor permissions from the enclosing AAS grant.
func ContextWithReBACEmbeddedRead(ctx context.Context) context.Context {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx
	}
	session := AuthorizationSessionFromContext(ctx)
	if session == nil {
		return ctx
	}
	view := session.semanticView(SemanticResourceSMDesc, SemanticResourceAASDesc)
	scoped := &AuthorizedQuery{outer: view, session: session, relatedViews: map[SemanticResourceKind]SemanticAccessView{}}
	ctx = context.WithValue(ctx, authorizedQueryContextKey{}, scoped)
	return context.WithValue(ctx, filterKey, view.queryFilter)
}

func (s *rebacSecurity) validateEmbeddedChildMutation(ctx context.Context, tx *sql.Tx, request *rebacRequest, aasID string, child rebac.StoredResource, live map[string]bool, deleted bool) error {
	var parts []string
	if err := json.Unmarshal([]byte(child.Identifier), &parts); err != nil || len(parts) != 2 {
		return rebac.ErrForbidden
	}
	if request.route.Kind == ReBACRouteKindSubmodelDescriptor && request.route.Identifier != parts[1] {
		return nil
	}
	method := http.MethodPut
	if deleted || !live[parts[1]] {
		method = http.MethodDelete
	}
	return s.authorizeEmbeddedMutation(ctx, tx, request, aasID, parts[1], method)
}
