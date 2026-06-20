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

package platform

import (
	"errors"
	"io"
	"net/http"

	"github.com/actordock/actordock/internal/store"
)

func (s *Server) handlePutSandboxNetwork(w http.ResponseWriter, r *http.Request) {
	sandboxID := r.PathValue("id")
	if sandboxID == "" {
		writeAPIError(w, http.StatusBadRequest, "sandbox id is required")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	upd, err := store.ParseNetworkUpdate(body)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := store.ValidateNetworkUpdate(upd); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	sb, err := s.store.Get(ctx, sandboxID)
	if errors.Is(err, store.ErrNotFound) {
		writeAPIError(w, http.StatusNotFound, "sandbox not found")
		return
	}
	if err != nil {
		s.logger.Error("get sandbox for network update", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to update sandbox network")
		return
	}

	store.ApplyNetworkUpdate(&sb, upd)
	if err := s.store.Put(ctx, sb); err != nil {
		s.logger.Error("persist sandbox network", "sandbox_id", sandboxID, "err", err)
		writeAPIError(w, http.StatusInternalServerError, "failed to update sandbox network")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
