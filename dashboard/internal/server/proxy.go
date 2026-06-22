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

package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const platformAPIPrefix = "/api/platform"
const routerAPIPrefix = "/api/router"

func newPlatformProxy(cfg Config, logger *slog.Logger) (http.Handler, error) {
	target, err := url.Parse(cfg.PlatformURL)
	if err != nil {
		return nil, fmt.Errorf("parse platform URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("platform proxy error", "err", err, "path", r.URL.Path)
		http.Error(w, "platform unreachable", http.StatusBadGateway)
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
		req.Header.Set("X-API-KEY", cfg.APIKey)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cfg.APIKey == "" {
			http.Error(w, "platform API key not configured", http.StatusServiceUnavailable)
			return
		}

		suffix := strings.TrimPrefix(r.URL.Path, platformAPIPrefix)
		if suffix == "" {
			suffix = "/"
		}
		out := r.Clone(r.Context())
		out.URL.Path = suffix
		out.URL.RawPath = ""
		proxy.ServeHTTP(w, out)
	}), nil
}

func newRouterProxy(cfg Config, logger *slog.Logger) (http.Handler, error) {
	target, err := url.Parse(cfg.RouterURL)
	if err != nil {
		return nil, fmt.Errorf("parse router URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = 100 * time.Millisecond
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error("router proxy error", "err", err, "path", r.URL.Path)
		http.Error(w, "router unreachable", http.StatusBadGateway)
	}
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		suffix := strings.TrimPrefix(r.URL.Path, routerAPIPrefix)
		if suffix == "" {
			suffix = "/"
		}
		out := r.Clone(r.Context())
		out.URL.Path = suffix
		out.URL.RawPath = ""
		proxy.ServeHTTP(w, out)
	}), nil
}
