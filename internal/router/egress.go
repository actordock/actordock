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

package router

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/actordock/actordock/internal/store"
)

var errInternetBlocked = errors.New("internet access disabled")

type sandboxPolicyReader interface {
	Get(ctx context.Context, sandboxID string) (store.Sandbox, error)
}

func isEgressRequest(r *http.Request) bool {
	if r.Method == http.MethodConnect {
		return true
	}
	return strings.HasPrefix(r.RequestURI, "http://") || strings.HasPrefix(r.RequestURI, "https://")
}

func egressUpstreamHost(r *http.Request) string {
	if r.Method == http.MethodConnect {
		return r.Host
	}
	targetURL, err := url.Parse(r.RequestURI)
	if err != nil {
		return ""
	}
	return targetURL.Host
}

func (s *Server) checkEgressAllowed(ctx context.Context, sandboxID, upstreamHost string) error {
	if store.IsInternalHost(upstreamHost) {
		return nil
	}
	if s.policies == nil {
		return nil
	}
	sb, err := s.policies.Get(ctx, sandboxID)
	if err != nil {
		return err
	}
	if !store.InternetAccessAllowed(sb) {
		return errInternetBlocked
	}
	return nil
}

func (s *Server) handleEgress(w http.ResponseWriter, r *http.Request, sandboxID string) {
	upstreamHost := egressUpstreamHost(r)
	if upstreamHost == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid proxy request")
		return
	}

	if err := s.checkEgressAllowed(r.Context(), sandboxID, upstreamHost); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeAPIError(w, http.StatusNotFound, "sandbox not found")
		case errors.Is(err, errInternetBlocked):
			writeAPIError(w, http.StatusForbidden, "internet access disabled")
		default:
			s.logger.Error("check egress policy", "sandbox_id", sandboxID, "err", err)
			writeAPIError(w, http.StatusInternalServerError, "failed to check network policy")
		}
		return
	}

	if r.Method == http.MethodConnect {
		s.handleEgressCONNECT(w, r)
		return
	}
	s.handleEgressHTTP(w, r)
}

func (s *Server) handleEgressHTTP(w http.ResponseWriter, r *http.Request) {
	targetURL, err := url.Parse(r.RequestURI)
	if err != nil || targetURL.Host == "" {
		writeAPIError(w, http.StatusBadRequest, "invalid proxy request")
		return
	}

	outReq := r.Clone(r.Context())
	outReq.URL = targetURL
	outReq.RequestURI = ""
	outReq.Host = targetURL.Host

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: targetURL.Scheme,
		Host:   targetURL.Host,
	})
	if s.egressTransport != nil {
		proxy.Transport = s.egressTransport
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
		s.logger.Error("egress proxy", "host", targetURL.Host, "err", err)
		writeAPIError(w, http.StatusBadGateway, "failed to reach upstream")
	}
	proxy.ServeHTTP(w, outReq)
}

func (s *Server) handleEgressCONNECT(w http.ResponseWriter, r *http.Request) {
	targetConn, err := net.Dial("tcp", r.Host)
	if err != nil {
		s.logger.Error("egress connect dial", "host", r.Host, "err", err)
		writeAPIError(w, http.StatusBadGateway, "failed to reach upstream")
		return
	}
	defer targetConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		writeAPIError(w, http.StatusInternalServerError, "connect hijack unsupported")
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		s.logger.Error("egress connect hijack", "host", r.Host, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to establish tunnel")
		return
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		s.logger.Error("egress connect response", "host", r.Host, "err", err)
		return
	}

	go func() {
		transfer(clientConn, targetConn)
	}()
	transfer(targetConn, clientConn)
}

func transfer(dst net.Conn, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if conn, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = conn.CloseWrite()
	} else {
		_ = dst.Close()
	}
	if conn, ok := src.(interface{ CloseRead() error }); ok {
		_ = conn.CloseRead()
	} else {
		_ = src.Close()
	}
}
