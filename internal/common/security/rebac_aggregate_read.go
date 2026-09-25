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
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/rebac"
)

type rebacReadTransactionKey struct{}

// ReBACPlanningContext permits private inclusion-set discovery. Its results must
// pass resource authorization and complete-representation validation before release.
func ReBACPlanningContext(ctx context.Context) context.Context {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx
	}
	ctx = context.WithValue(ctx, filterKey, (*QueryFilter)(nil))
	ctx = context.WithValue(ctx, authorizationSessionContextKey{}, (*AuthorizationSession)(nil))
	return context.WithValue(ctx, authorizedQueryContextKey{}, (*AuthorizedQuery)(nil))
}

// AuthorizeReBACReadResource creates an independently authorized context for one included member.
func AuthorizeReBACReadResource(ctx context.Context, kind, id string) (context.Context, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx, nil
	}
	path, err := rebacCanonicalPath(kind, id)
	if err != nil {
		return ctx, err
	}
	return authorizeReBACReadPath(ctx, path)
}

func authorizeReBACReadPath(ctx context.Context, path string) (context.Context, error) {
	return authorizeReBACPath(ctx, path, http.MethodGet)
}

func authorizeReBACPath(ctx context.Context, path, method string) (context.Context, error) {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return ctx, nil
	}
	original, ok := ctx.Value(rebacRequestKey{}).(*rebacRequest)
	if !ok || original.security == nil {
		return ctx, common.NewErrServiceUnavailable("REBAC-AGGREGATE-CONTEXT initiating authorization is required")
	}
	s := original.security
	route, err := ClassifyReBACRoute(method, path)
	if err != nil {
		return ctx, err
	}
	request := *original
	request.route = route
	ctx = context.WithValue(ctx, rebacRequestKey{}, &request)
	path = strings.TrimRight(common.NormalizeBasePath(cfg.Server.ContextPath), "/") + path
	synthetic := (&http.Request{Method: method, URL: &url.URL{Path: path}}).WithContext(ctx)
	var result *http.Request
	check := func(tx *sql.Tx) error {
		state, lockErr := s.runtime.State.Lock(ctx, tx, false)
		if lockErr != nil {
			return lockErr
		}
		if state.Applied != original.revision {
			return rebac.ErrStaleRevision
		}
		result, lockErr = s.authorizeRequest(synthetic, tx, &request)
		return lockErr
	}
	if tx, ok := ctx.Value(rebacReadTransactionKey{}).(*sql.Tx); ok && tx != nil {
		err = check(tx)
	} else {
		err = s.runtime.State.WithApplied(ctx, check)
	}
	if errors.Is(err, rebac.ErrForbidden) {
		return ctx, common.NewErrDenied("REBAC-AGGREGATE-DENIED member access denied")
	}
	if err != nil {
		return ctx, common.NewErrServiceUnavailable(err.Error())
	}
	return result.Context(), nil
}

// RequireCompleteReBACRepresentation rejects aggregates whose authorized projection omits content.
func RequireCompleteReBACRepresentation(ctx context.Context, complete, visible any) error {
	cfg, ok := common.ConfigFromContext(ctx)
	if !ok || !cfg.ReBAC.Enabled {
		return nil
	}
	raw, err := rebacRepresentationBytes(complete)
	if err != nil {
		return err
	}
	filtered, err := rebacRepresentationBytes(visible)
	if err != nil {
		return err
	}
	if !bytes.Equal(raw, filtered) {
		return common.NewErrDenied("REBAC-AGGREGATE-INCOMPLETE complete representation is not authorized")
	}
	return nil
}

func rebacRepresentationBytes(value any) ([]byte, error) {
	if model, ok := value.(types.IClass); ok {
		converted, err := jsonization.ToJsonable(model)
		if err != nil {
			return nil, err
		}
		return json.Marshal(converted)
	}
	return json.Marshal(value)
}
