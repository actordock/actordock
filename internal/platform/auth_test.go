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
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestListAPIKeysIncludesBootstrap(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "/api-keys", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []teamAPIKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].Name != "default" || resp[0].ID == "" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp[0].CreatedBy != nil || resp[0].LastUsed != nil {
		t.Fatalf("nullable fields = %+v", resp[0])
	}
	if resp[0].Mask.ValueLength != 3 {
		t.Fatalf("mask = %+v", resp[0].Mask)
	}
}

func TestCreateAPIKeyAndAuthenticate(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"name":"ci-bot"}`)
	req := httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var created createdTeamAPIKeyResponse
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Key == "" || created.Name != "ci-bot" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	if created.Mask.ValueLength != len(created.Key) {
		t.Fatalf("mask = %+v", created.Mask)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api-keys", nil)
	listReq.Header.Set("X-API-KEY", created.Key)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list with new key status = %d", listRec.Code)
	}
	var listed []teamAPIKeyResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("listed len = %d", len(listed))
	}
}

func TestCreateAccessTokenAndDelete(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())

	createBody := []byte(`{"name":"dashboard"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/access-tokens", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-KEY", "dev")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created createdAccessTokenResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.Token == "" || created.Name != "dashboard" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
	if created.Mask.ValueLength != len(created.Token) {
		t.Fatalf("mask = %+v", created.Mask)
	}

	delReq := httptest.NewRequest(http.MethodDelete, "/access-tokens/"+created.ID, nil)
	delReq.Header.Set("X-API-KEY", "dev")
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", delRec.Code)
	}

	delReq2 := httptest.NewRequest(http.MethodDelete, "/access-tokens/"+created.ID, nil)
	delReq2.Header.Set("X-API-KEY", "dev")
	delRec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec2, delReq2)
	if delRec2.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d", delRec2.Code)
	}
}

func TestCreateAPIKeyUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/api-keys", bytes.NewReader([]byte(`{"name":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
