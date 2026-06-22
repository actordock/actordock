// Copyright 2026 The Actordock Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package envd

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
)

const accessTokenHeader = "X-Access-Token"

// InsecureEnvVar disables envd access-token checks when set to "true" (Kind dev escape hatch).
const InsecureEnvVar = "ACTORDOCK_ENVD_INSECURE"

type accessGuard struct {
	mu    sync.RWMutex
	token string
}

func (g *accessGuard) setToken(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = strings.TrimSpace(token)
}

func (g *accessGuard) clearToken() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.token = ""
}

func (g *accessGuard) required() bool {
	if strings.EqualFold(os.Getenv(InsecureEnvVar), "true") {
		return false
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.token != ""
}

func (g *accessGuard) validate(provided string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if g.token == "" {
		return true
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(g.token)) == 1
}

func (g *accessGuard) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.required() || isAccessExemptPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		if !g.validate(r.Header.Get(accessTokenHeader)) {
			writeAccessError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAccessExemptPath(r *http.Request) bool {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/health":
		return true
	case r.Method == http.MethodPost && r.URL.Path == "/init":
		return true
	default:
		return false
	}
}

type initRequest struct {
	AccessToken string `json:"accessToken"`
}

func (g *accessGuard) handleInit(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req initRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	if token := strings.TrimSpace(req.AccessToken); token != "" {
		g.setToken(token)
	} else {
		g.clearToken()
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeAccessError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    code,
		"message": err.Error(),
	})
}
