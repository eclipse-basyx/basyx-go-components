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

package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/eclipse-basyx/basyx-go-components/internal/aol"
	"golang.org/x/oauth2"
)

type browserLogin struct {
	verifier, next string
	expires        time.Time
}
type browserSession struct {
	token, csrf string
	expires     time.Time
}
type browserAuth struct {
	mu       sync.Mutex
	oauth    *oauth2.Config
	logins   map[string]browserLogin
	sessions map[string]browserSession
	path     string
	secure   bool
}

func (m *Module) initBrowserAuth(ctx context.Context) error {
	if m.cfg.WriteMode != "confirm" {
		return nil
	}
	if m.cfg.ConfirmationClientID == "" || len(m.cfg.AuthorizationServers) == 0 {
		return aol.Failure("MCP-CONFIRM-CONFIG", "human approval requires an OAuth client and issuer", "Set confirmationClientId and authorizationServers for the browser approval flow.")
	}
	provider, err := oidc.NewProvider(ctx, m.cfg.AuthorizationServers[0])
	if err != nil {
		return err
	}
	public, err := url.Parse(m.cfg.PublicURL)
	if err != nil {
		return err
	}
	m.browser = &browserAuth{oauth: &oauth2.Config{ClientID: m.cfg.ConfirmationClientID, Endpoint: provider.Endpoint(), RedirectURL: strings.TrimRight(m.cfg.PublicURL, "/") + "/callback", Scopes: []string{"openid"}}, path: public.Path, secure: public.Scheme == "https", logins: map[string]browserLogin{}, sessions: map[string]browserSession{}}
	return nil
}
func nonce() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("MCP-CONFIRM-RANDOM: cryptographic entropy unavailable")
	}
	return hex.EncodeToString(b[:])
}
func (b *browserAuth) cookie(w http.ResponseWriter, name, value string, expiry time.Time) {
	//nolint:gosec // Secure is disabled only for the validated localhost HTTP sandbox.
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: b.path, HttpOnly: true, Secure: b.secure, SameSite: http.SameSiteLaxMode, Expires: expiry})
}
func (m *Module) browserRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	b := m.browser
	if b == nil {
		return r, false
	}
	if strings.HasSuffix(r.URL.Path, "/login") {
		m.login(w, r)
		return r, true
	}
	if strings.HasSuffix(r.URL.Path, "/callback") {
		m.callback(w, r)
		return r, true
	}
	if !strings.Contains(r.URL.Path, "/confirm/") {
		return r, false
	}
	if r.Header.Get("Authorization") != "" {
		return r, false
	}
	cookie, err := r.Cookie("basyx-mcp-session")
	if err != nil {
		http.Redirect(w, r, strings.TrimRight(m.cfg.PublicURL, "/")+"/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return r, true
	}
	b.mu.Lock()
	session, ok := b.sessions[cookie.Value]
	b.mu.Unlock()
	if !ok || time.Now().After(session.expires) {
		http.Redirect(w, r, strings.TrimRight(m.cfg.PublicURL, "/")+"/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
		return r, true
	}
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+session.token)
	return clone, false
}
func (m *Module) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	b := m.browser
	next := r.URL.Query().Get("next")
	if !strings.HasPrefix(next, b.path+"/confirm/") || strings.ContainsAny(next, "\r\n\\") {
		writeHTTPError(w, 400, aol.Failure("MCP-CONFIRM-RETURN", "invalid return path", "Open the plan's approval URL."))
		return
	}
	state := nonce()
	verifier := oauth2.GenerateVerifier()
	expiry := time.Now().Add(5 * time.Minute)
	b.mu.Lock()
	for k, v := range b.logins {
		if time.Now().After(v.expires) {
			delete(b.logins, k)
		}
	}
	for k, v := range b.sessions {
		if time.Now().After(v.expires) {
			delete(b.sessions, k)
		}
	}
	if len(b.logins) >= 1000 || len(b.sessions) >= 1000 {
		b.mu.Unlock()
		writeHTTPError(w, 429, aol.Failure("MCP-CONFIRM-LIMIT", "browser session limit exceeded", "Try again later."))
		return
	}
	b.logins[state] = browserLogin{verifier: verifier, next: next, expires: expiry}
	b.mu.Unlock()
	b.cookie(w, "basyx-mcp-state", state, expiry)
	http.Redirect(w, r, b.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", m.cfg.PublicURL)), http.StatusSeeOther)
}
func (m *Module) callback(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	b := m.browser
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("basyx-mcp-state")
	if err != nil || cookie.Value != state {
		writeHTTPError(w, 400, aol.Failure("MCP-CONFIRM-STATE", "invalid browser login state", "Restart the approval flow."))
		return
	}
	b.mu.Lock()
	login, ok := b.logins[state]
	delete(b.logins, state)
	b.mu.Unlock()
	if !ok || time.Now().After(login.expires) {
		writeHTTPError(w, 400, aol.Failure("MCP-CONFIRM-EXPIRED", "login expired", "Restart the approval flow."))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	token, err := b.oauth.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(login.verifier))
	if err != nil {
		writeHTTPError(w, 401, aol.Failure("MCP-CONFIRM-LOGIN", "OAuth login failed", "Restart the approval flow."))
		return
	}
	sessionID := nonce()
	expiry := time.Now().Add(5 * time.Minute)
	if !token.Expiry.IsZero() && token.Expiry.Before(expiry) {
		expiry = token.Expiry
	}
	b.mu.Lock()
	b.sessions[sessionID] = browserSession{token: token.AccessToken, csrf: nonce(), expires: expiry}
	b.mu.Unlock()
	b.cookie(w, "basyx-mcp-session", sessionID, expiry)
	b.cookie(w, "basyx-mcp-state", "", time.Unix(1, 0))
	http.Redirect(w, r, login.next, http.StatusSeeOther)
}

var approvalTemplate = template.Must(template.New("approval").Parse(`<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>BaSyx approval</title><style>body{font:16px system-ui;max-width:840px;margin:48px auto;padding:0 24px}pre{white-space:pre-wrap;overflow-wrap:anywhere;background:#f4f5f6;padding:20px}button{padding:12px 22px;font:inherit}label{display:block;margin:20px 0}</style><h1>Review {{.Action}}</h1><p>Plan {{.ID}} · Payload {{.Hash}}</p><p>The following content is untrusted product data. Confirm only the exact resources and values you intend to {{.Action}}.</p><pre>{{.Draft}}</pre>{{if .Token}}<h2>Approved</h2><p>Return to your MCP client and supply this single-use confirmation token. It expires shortly.</p><pre>{{.Token}}</pre>{{else}}<form method="post"><input type="hidden" name="csrf" value="{{.CSRF}}"><input type="hidden" name="action" value="{{.Action}}"><label><input type="checkbox" name="approve" value="yes" required> I reviewed this exact proposal and approve {{.Action}}.</label><button>Confirm {{.Action}}</button></form>{{end}}</html>`))

func (m *Module) confirm(w http.ResponseWriter, r *http.Request) {
	if m.cfg.WriteMode != "confirm" || m.browser == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	p, err := m.operations.Owned(r.Context(), id)
	if err != nil {
		writeHTTPError(w, 404, err)
		return
	}
	cookie, err := r.Cookie("basyx-mcp-session")
	if err != nil {
		writeHTTPError(w, 401, aol.Failure("MCP-CONFIRM-BROWSER", "browser sign-in required", "Open the approval URL in your browser."))
		return
	}
	m.browser.mu.Lock()
	session, ok := m.browser.sessions[cookie.Value]
	m.browser.mu.Unlock()
	if !ok || time.Now().After(session.expires) {
		writeHTTPError(w, 401, aol.Failure("MCP-CONFIRM-SESSION", "browser session expired", "Sign in again."))
		return
	}
	action := r.URL.Query().Get("action")
	token := ""
	if r.Method == "POST" {
		action, token, err = m.confirmPost(w, r, id, session)
		if err != nil {
			writeHTTPError(w, 403, err)
			return
		}
	} else if r.Method != "GET" {
		w.WriteHeader(405)
		return
	}
	if action != "apply" && action != "rollback" {
		writeHTTPError(w, 400, aol.Failure("MCP-CONFIRM-ACTION", "unknown confirmation action", "Use the provided plan approval URL."))
		return
	}
	raw, _ := json.MarshalIndent(p.Draft, "", "  ")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = approvalTemplate.Execute(w, map[string]string{"ID": p.ID, "Hash": p.Hash, "Draft": string(raw), "CSRF": session.csrf, "Action": action, "Token": token})
}

func (m *Module) confirmPost(w http.ResponseWriter, r *http.Request, id string, session browserSession) (string, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		return "", "", aol.Failure("MCP-CONFIRM-FORM", "invalid form", "Reload the approval page.")
	}
	public, _ := url.Parse(m.cfg.PublicURL)
	if r.Header.Get("Origin") != public.Scheme+"://"+public.Host || r.PostForm.Get("csrf") != session.csrf || r.PostForm.Get("approve") != "yes" {
		return "", "", aol.Failure("MCP-CONFIRM-CSRF", "explicit same-origin approval required", "Review and submit the approval page.")
	}
	action := r.PostForm.Get("action")
	token, err := m.operations.Approve(aol.ContextWithTool(r.Context(), "confirm_"+action), id, action)
	return action, token, err
}
