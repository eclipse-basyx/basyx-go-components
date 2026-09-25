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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

type rebacWriterTransactionKey struct{}

// AuthorizeReBACResource checks a concrete aggregate member in its writer transaction.
// The returned context is valid only for that member; callers must not reuse it for other resources.
func AuthorizeReBACResource(ctx context.Context, tx *sql.Tx, kind, id, method string) (context.Context, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx, nil
	}
	path, err := rebacCanonicalPath(kind, id)
	if err != nil {
		return ctx, err
	}
	return authorizeReBACMutationPath(ctx, tx, path, method)
}

func authorizeReBACMutationPath(ctx context.Context, tx *sql.Tx, path, method string) (context.Context, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx, nil
	}
	original, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || original.security == nil || tx == nil {
		return ctx, common.NewErrServiceUnavailable("REBAC-RESOURCE-CONTEXT validated request and transaction required")
	}
	s := original.security
	if err := s.runtime.State.CheckMutationRevision(ctx, tx, original.revision); err != nil {
		return ctx, common.NewErrServiceUnavailable(err.Error())
	}
	path = strings.TrimRight(common.NormalizeBasePath(cfg.Server.ContextPath), "/") + path
	request := *original
	route, err := ClassifyReBACRoute(method, pathWithoutBase(path, cfg.Server.ContextPath))
	if err != nil {
		return ctx, err
	}
	request.route = route
	ctx = context.WithValue(ctx, rebacRequestKey{}, &request)
	ctx = context.WithValue(ctx, rebacWriterTransactionKey{}, tx)
	synthetic := &http.Request{Method: method, URL: &url.URL{Path: path}}
	synthetic = synthetic.WithContext(ctx)
	authorized, err := s.authorizeRequest(synthetic, tx, &request)
	if errors.Is(err, rebac.ErrForbidden) {
		return ctx, common.NewErrDenied("REBAC-RESOURCE-DENIED resource access denied")
	}
	if err != nil {
		return ctx, common.NewErrServiceUnavailable(err.Error())
	}
	return authorized.Context(), nil
}

func rebacCanonicalPath(kind, id string) (string, error) {
	prefixes := map[string]string{"aas": "/shells/", "submodel": "/submodels/", "aas_descriptor": "/shell-descriptors/", "submodel_descriptor": "/submodel-descriptors/", "concept_description": "/concept-descriptions/", "discovery": "/lookup/shells/", "package": "/packages/"}
	prefix, ok := prefixes[kind]
	if !ok {
		return "", fmt.Errorf("REBAC-RESOURCE-KIND unsupported aggregate member")
	}
	return prefix + common.EncodeString(id), nil
}

func pathWithoutBase(path, base string) string {
	base = strings.TrimRight(common.NormalizeBasePath(base), "/")
	if base != "" && len(path) > len(base) {
		return path[len(base):]
	}
	return path
}

func (s *rebacSecurity) serveResourceAggregate(w http.ResponseWriter, r *http.Request, next http.Handler, request *rebacRequest) {
	var prepared *http.Request
	request.fallbackMethod, request.fallbackPath = r.Method, r.URL.Path
	err := s.runtime.State.WithApplied(r.Context(), func(tx *sql.Tx) error {
		state, err := s.runtime.State.Lock(r.Context(), tx, false)
		if err != nil {
			return err
		}
		request.revision = state.Applied
		model := activeAccessModel(s.settings)
		if model == nil {
			return rebac.ErrForbidden
		}
		options := grammar.DefaultSimplifyOptions()
		options.EnableImplicitCasts = s.settings.EnableImplicitCasts
		session := newAuthorizationSession(model, ClaimsFromContext(r.Context()), nil, options)
		ctx := context.WithValue(r.Context(), authorizationSessionContextKey{}, session)
		user := ""
		if request.identity.Principal != nil {
			user = request.actor.User
		}
		ctx = rebac.ContextWithLifecycleActor(ctx, rebac.LifecycleActor{User: user, Revision: request.revision})
		prepared = r.WithContext(ctx)
		if request.route.Action == ReBACRouteActionRead {
			prepared = prepared.WithContext(context.WithValue(ctx, rebacReadTransactionKey{}, tx))
			next.ServeHTTP(w, prepared)
		}
		return nil
	})
	if err != nil {
		writeManagementError(w, err)
		return
	}
	if request.route.Action != ReBACRouteActionRead {
		next.ServeHTTP(w, prepared)
	}
}
