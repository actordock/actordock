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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/actordock/actordock/internal/config"
	"github.com/actordock/actordock/internal/envd"
	"github.com/actordock/actordock/internal/runtimeapi"
	"github.com/actordock/actordock/internal/store"
	processv1 "github.com/actordock/actordock/pkg/envd/process"
	"github.com/actordock/actordock/pkg/envd/process/processv1connect"
	"github.com/actordock/runtime/pkg/proto/runtimeapipb"
)

type fakeActors struct {
	lastActorID           string
	lastTemplateNamespace string
	lastTemplateName      string
	lastDeletedID         string
	lastSuspended         string
	lastResumed           string
	lastSnapshotActor     string
	backendAddr           string
	createErr             error
	deleteErr             error
	suspendErr            error
	resumeErr             error
	createSnapshotErr     error
	getErr                error
	backendErr            error
	snapshotResult        runtimeapi.SnapshotResult
	actorStatuses         map[string]runtimeapipb.Actor_Status
	defaultStatus         runtimeapipb.Actor_Status
}

func (f *fakeActors) CreateAndResumeSandbox(_ context.Context, actorID, templateNamespace, templateName string) error {
	f.lastActorID = actorID
	f.lastTemplateNamespace = templateNamespace
	f.lastTemplateName = templateName
	return f.createErr
}

func (f *fakeActors) DeleteSandbox(_ context.Context, actorID string) error {
	f.lastDeletedID = actorID
	return f.deleteErr
}

func (f *fakeActors) SuspendSandbox(_ context.Context, actorID string) error {
	f.lastSuspended = actorID
	return f.suspendErr
}

func (f *fakeActors) ResumeSandbox(_ context.Context, actorID string) error {
	f.lastResumed = actorID
	return f.resumeErr
}

func (f *fakeActors) CreateSnapshot(_ context.Context, actorID string) (runtimeapi.SnapshotResult, error) {
	f.lastSnapshotActor = actorID
	if f.createSnapshotErr != nil {
		return runtimeapi.SnapshotResult{}, f.createSnapshotErr
	}
	if f.snapshotResult.SnapshotURI == "" {
		return runtimeapi.SnapshotResult{
			SnapshotURI:  "gs://bucket/actordock/" + actorID + "/snap",
			SnapshotType: "SNAPSHOT_TYPE_EXTERNAL",
		}, nil
	}
	return f.snapshotResult, nil
}

func (f *fakeActors) GetActor(_ context.Context, actorID string) (runtimeapipb.Actor_Status, error) {
	if f.getErr != nil {
		return runtimeapipb.Actor_STATUS_UNSPECIFIED, f.getErr
	}
	if f.actorStatuses != nil {
		if status, ok := f.actorStatuses[actorID]; ok {
			return status, nil
		}
	}
	if f.defaultStatus != runtimeapipb.Actor_STATUS_UNSPECIFIED {
		return f.defaultStatus, nil
	}
	return runtimeapipb.Actor_STATUS_RUNNING, nil
}

func (f *fakeActors) GetActorBackend(_ context.Context, actorID string, _ int) (string, error) {
	if f.backendErr != nil {
		return "", f.backendErr
	}
	if f.backendAddr == "" {
		return "", fmt.Errorf("actor %q has no worker assigned", actorID)
	}
	return f.backendAddr, nil
}

func (f *fakeActors) ResumeSandboxBackend(ctx context.Context, actorID string, envdPort int) (string, bool, error) {
	backend, err := f.GetActorBackend(ctx, actorID, envdPort)
	return backend, false, err
}

type fakeStore struct {
	records             map[string]store.Sandbox
	snapshots           map[string]store.Snapshot
	volumes             map[string]store.Volume
	volNames            map[string]string
	catalogTemplates    map[string]store.CatalogTemplateRecord
	templateBuilds      map[string]store.TemplateBuild
	templateBuildLatest map[string]string
	templateBuildFiles  map[string]store.TemplateBuildFile
	templateBuildQueue  []store.TemplateBuildJob
	buildLogs           []store.BuildLogEntry
	templateTags        map[string]store.TemplateTagRecord
	teamAPIKeys         map[string]store.TeamAPIKeyRecord
	apiKeyHashes        map[string]string
	userAccessTokens    map[string]store.UserAccessTokenRecord
	putErr              error
	delErr              error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		records:             make(map[string]store.Sandbox),
		snapshots:           make(map[string]store.Snapshot),
		volumes:             make(map[string]store.Volume),
		volNames:            make(map[string]string),
		catalogTemplates:    make(map[string]store.CatalogTemplateRecord),
		templateBuilds:      make(map[string]store.TemplateBuild),
		templateBuildLatest: make(map[string]string),
		templateBuildFiles:  make(map[string]store.TemplateBuildFile),
		templateTags:        make(map[string]store.TemplateTagRecord),
		teamAPIKeys:         make(map[string]store.TeamAPIKeyRecord),
		apiKeyHashes:        make(map[string]string),
		userAccessTokens:    make(map[string]store.UserAccessTokenRecord),
		putErr:              nil,
		delErr:              nil,
	}
}

func (f *fakeStore) Put(_ context.Context, sb store.Sandbox) error {
	if f.putErr != nil {
		return f.putErr
	}
	f.records[sb.SandboxID] = sb
	return nil
}

func (f *fakeStore) Delete(_ context.Context, sandboxID string) error {
	if f.delErr != nil {
		return f.delErr
	}
	delete(f.records, sandboxID)
	return nil
}

func (f *fakeStore) Get(_ context.Context, sandboxID string) (store.Sandbox, error) {
	sb, ok := f.records[sandboxID]
	if !ok {
		return store.Sandbox{}, store.ErrNotFound
	}
	return sb, nil
}

func (f *fakeStore) List(_ context.Context) ([]store.Sandbox, error) {
	out := make([]store.Sandbox, 0, len(f.records))
	for _, sb := range f.records {
		out = append(out, sb)
	}
	return out, nil
}

func (f *fakeStore) PutSnapshot(_ context.Context, snap store.Snapshot) error {
	f.snapshots[snap.SnapshotID] = snap
	return nil
}

func (f *fakeStore) GetSnapshot(_ context.Context, snapshotID string) (store.Snapshot, error) {
	snap, ok := f.snapshots[snapshotID]
	if !ok {
		return store.Snapshot{}, store.ErrSnapshotNotFound
	}
	return snap, nil
}

func (f *fakeStore) ListSnapshots(_ context.Context) ([]store.Snapshot, error) {
	out := make([]store.Snapshot, 0, len(f.snapshots))
	for _, snap := range f.snapshots {
		out = append(out, snap)
	}
	return out, nil
}

func (f *fakeStore) PutVolume(_ context.Context, vol store.Volume) error {
	if existing, ok := f.volNames[vol.Name]; ok && existing != vol.VolumeID {
		return store.ErrVolumeNameTaken
	}
	f.volumes[vol.VolumeID] = vol
	f.volNames[vol.Name] = vol.VolumeID
	return nil
}

func (f *fakeStore) GetVolume(_ context.Context, volumeID string) (store.Volume, error) {
	vol, ok := f.volumes[volumeID]
	if !ok {
		return store.Volume{}, store.ErrVolumeNotFound
	}
	return vol, nil
}

func (f *fakeStore) GetVolumeByName(_ context.Context, name string) (store.Volume, error) {
	volumeID, ok := f.volNames[name]
	if !ok {
		return store.Volume{}, store.ErrVolumeNotFound
	}
	return f.GetVolume(context.Background(), volumeID)
}

func (f *fakeStore) ListVolumes(_ context.Context) ([]store.Volume, error) {
	out := make([]store.Volume, 0, len(f.volumes))
	for _, vol := range f.volumes {
		out = append(out, vol)
	}
	return out, nil
}

func (f *fakeStore) DeleteVolume(_ context.Context, volumeID string) error {
	vol, ok := f.volumes[volumeID]
	if !ok {
		return store.ErrVolumeNotFound
	}
	delete(f.volumes, volumeID)
	delete(f.volNames, vol.Name)
	return nil
}

func (f *fakeStore) PutCatalogTemplate(_ context.Context, rec store.CatalogTemplateRecord) error {
	if _, ok := f.catalogTemplates[rec.TemplateID]; ok {
		return store.ErrCatalogTemplateExists
	}
	f.catalogTemplates[rec.TemplateID] = rec
	return nil
}

func (f *fakeStore) GetCatalogTemplate(_ context.Context, templateID string) (store.CatalogTemplateRecord, error) {
	rec, ok := f.catalogTemplates[templateID]
	if !ok {
		return store.CatalogTemplateRecord{}, store.ErrCatalogTemplateNotFound
	}
	return rec, nil
}

func (f *fakeStore) ListCatalogTemplates(_ context.Context) ([]store.CatalogTemplateRecord, error) {
	out := make([]store.CatalogTemplateRecord, 0, len(f.catalogTemplates))
	for _, rec := range f.catalogTemplates {
		out = append(out, rec)
	}
	return out, nil
}

func (f *fakeStore) UpdateCatalogTemplate(_ context.Context, rec store.CatalogTemplateRecord) error {
	if _, ok := f.catalogTemplates[rec.TemplateID]; !ok {
		return store.ErrCatalogTemplateNotFound
	}
	f.catalogTemplates[rec.TemplateID] = rec
	return nil
}

func templateBuildMapKey(templateID, buildID string) string {
	return templateID + ":" + buildID
}

func (f *fakeStore) PutTemplateBuild(_ context.Context, build store.TemplateBuild) error {
	f.templateBuilds[templateBuildMapKey(build.TemplateID, build.BuildID)] = build
	f.templateBuildLatest[build.TemplateID] = build.BuildID
	return nil
}

func (f *fakeStore) GetTemplateBuild(_ context.Context, templateID, buildID string) (store.TemplateBuild, error) {
	build, ok := f.templateBuilds[templateBuildMapKey(templateID, buildID)]
	if !ok {
		return store.TemplateBuild{}, store.ErrTemplateBuildNotFound
	}
	return build, nil
}

func (f *fakeStore) UpdateTemplateBuild(_ context.Context, build store.TemplateBuild) error {
	key := templateBuildMapKey(build.TemplateID, build.BuildID)
	if _, ok := f.templateBuilds[key]; !ok {
		return store.ErrTemplateBuildNotFound
	}
	f.templateBuilds[key] = build
	return nil
}

func (f *fakeStore) ListTemplateBuilds(_ context.Context, templateID string) ([]store.TemplateBuild, error) {
	out := make([]store.TemplateBuild, 0)
	for _, build := range f.templateBuilds {
		if build.TemplateID == templateID {
			out = append(out, build)
		}
	}
	return out, nil
}

func (f *fakeStore) GetLatestTemplateBuild(_ context.Context, templateID string) (store.TemplateBuild, error) {
	buildID, ok := f.templateBuildLatest[templateID]
	if !ok {
		return store.TemplateBuild{}, store.ErrTemplateBuildNotFound
	}
	return f.GetTemplateBuild(context.Background(), templateID, buildID)
}

func (f *fakeStore) ListLatestTemplateBuilds(_ context.Context) ([]store.TemplateBuild, error) {
	out := make([]store.TemplateBuild, 0, len(f.templateBuildLatest))
	for templateID, buildID := range f.templateBuildLatest {
		build, err := f.GetTemplateBuild(context.Background(), templateID, buildID)
		if err != nil {
			return nil, err
		}
		out = append(out, build)
	}
	return out, nil
}

func (f *fakeStore) AppendBuildLog(_ context.Context, entry store.BuildLogEntry) error {
	f.buildLogs = append(f.buildLogs, entry)
	return nil
}

func (f *fakeStore) ListBuildLogs(_ context.Context, templateID, buildID string, offset, limit int) ([]store.BuildLogEntry, error) {
	var out []store.BuildLogEntry
	for _, entry := range f.buildLogs {
		if entry.TemplateID == templateID && entry.BuildID == buildID {
			out = append(out, entry)
		}
	}
	if offset > len(out) {
		return nil, nil
	}
	out = out[offset:]
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func templateTagMapKey(templateID, tag string) string {
	return templateID + "\x00" + tag
}

func (f *fakeStore) PutTemplateTag(_ context.Context, rec store.TemplateTagRecord) error {
	f.templateTags[templateTagMapKey(rec.TemplateID, rec.Tag)] = rec
	return nil
}

func (f *fakeStore) GetTemplateTag(_ context.Context, templateID, tag string) (store.TemplateTagRecord, error) {
	rec, ok := f.templateTags[templateTagMapKey(templateID, tag)]
	if !ok {
		return store.TemplateTagRecord{}, store.ErrTemplateTagNotFound
	}
	return rec, nil
}

func (f *fakeStore) ListTemplateTags(_ context.Context, templateID string) ([]store.TemplateTagRecord, error) {
	out := make([]store.TemplateTagRecord, 0)
	for _, rec := range f.templateTags {
		if rec.TemplateID == templateID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteTemplateTags(_ context.Context, templateID string, tags []string) error {
	for _, tag := range tags {
		delete(f.templateTags, templateTagMapKey(templateID, tag))
	}
	return nil
}

func (f *fakeStore) EnqueueTemplateBuild(_ context.Context, job store.TemplateBuildJob) error {
	f.templateBuildQueue = append(f.templateBuildQueue, job)
	return nil
}

func (f *fakeStore) EnqueueTemplateTagSync(_ context.Context, templateID, buildID, tag string) error {
	return f.EnqueueTemplateBuild(context.Background(), store.TemplateBuildJob{
		TemplateID: templateID,
		BuildID:    buildID,
		SyncTag:    tag,
		EnqueuedAt: time.Now().UTC(),
	})
}

func (f *fakeStore) PutTemplateBuildFile(_ context.Context, file store.TemplateBuildFile) error {
	f.templateBuildFiles[file.FilesHash] = file
	return nil
}

func (f *fakeStore) GetTemplateBuildFile(_ context.Context, filesHash string) (store.TemplateBuildFile, error) {
	file, ok := f.templateBuildFiles[filesHash]
	if !ok {
		return store.TemplateBuildFile{}, store.ErrTemplateBuildFileNotFound
	}
	return file, nil
}

func (f *fakeStore) MarkTemplateBuildFilePresent(_ context.Context, filesHash string, present bool) error {
	file, ok := f.templateBuildFiles[filesHash]
	if !ok {
		return store.ErrTemplateBuildFileNotFound
	}
	file.Present = present
	f.templateBuildFiles[filesHash] = file
	return nil
}

func (f *fakeStore) PutTeamAPIKey(_ context.Context, rec store.TeamAPIKeyRecord) error {
	if _, ok := f.apiKeyHashes[rec.KeyHash]; ok {
		return fmt.Errorf("duplicate api key hash")
	}
	f.teamAPIKeys[rec.ID] = rec
	f.apiKeyHashes[rec.KeyHash] = rec.ID
	return nil
}

func (f *fakeStore) ListTeamAPIKeys(_ context.Context) ([]store.TeamAPIKeyRecord, error) {
	out := make([]store.TeamAPIKeyRecord, 0, len(f.teamAPIKeys))
	for _, rec := range f.teamAPIKeys {
		out = append(out, rec)
	}
	return out, nil
}

func (f *fakeStore) ValidateTeamAPIKey(_ context.Context, raw string) (bool, error) {
	_, ok := f.apiKeyHashes[store.HashAPIKey(raw)]
	return ok, nil
}

func (f *fakeStore) PutUserAccessToken(_ context.Context, rec store.UserAccessTokenRecord) error {
	f.userAccessTokens[rec.ID] = rec
	return nil
}

func (f *fakeStore) DeleteUserAccessToken(_ context.Context, id string) error {
	if _, ok := f.userAccessTokens[id]; !ok {
		return store.ErrUserAccessTokenNotFound
	}
	delete(f.userAccessTokens, id)
	return nil
}

func TestHealth(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestSandboxCreateGetListFieldParity(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	createBody := []byte(`{
		"templateID":"base",
		"metadata":{"team":"acme","env":"dev"},
		"envVars":{"FOO":"bar"},
		"mcp":{"server":"demo"},
		"allow_internet_access":false,
		"network":{"allowOut":["1.1.1.1"]}
	}`)
	createReq := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(createBody))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-KEY", "dev")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	var created sandboxResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Alias != "base" {
		t.Fatalf("alias = %q, want base", created.Alias)
	}
	if created.SandboxID == "" {
		t.Fatal("sandboxID is empty")
	}

	stored := st.records[created.SandboxID]
	if stored.Metadata["team"] != "acme" || stored.Metadata["env"] != "dev" {
		t.Fatalf("metadata = %+v", stored.Metadata)
	}
	if stored.EnvVars["FOO"] != "bar" {
		t.Fatalf("envVars = %+v", stored.EnvVars)
	}
	if string(stored.Mcp) != `{"server":"demo"}` {
		t.Fatalf("mcp = %s", stored.Mcp)
	}
	if stored.AllowInternetAccess == nil || *stored.AllowInternetAccess {
		t.Fatalf("allow_internet_access = %v", stored.AllowInternetAccess)
	}
	if stored.Network == nil || len(stored.Network.AllowOut) != 1 {
		t.Fatalf("network = %+v", stored.Network)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/"+created.SandboxID, nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var detail sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if detail.Metadata["team"] != "acme" {
		t.Fatalf("detail metadata = %+v", detail.Metadata)
	}
	if detail.Alias != "base" || detail.Domain != "localhost" {
		t.Fatalf("detail alias/domain = %q/%q", detail.Alias, detail.Domain)
	}
	if detail.AllowInternetAccess == nil || *detail.AllowInternetAccess {
		t.Fatalf("detail allowInternetAccess = %v", detail.AllowInternetAccess)
	}
	if detail.Network == nil || detail.Network.AllowOut[0] != "1.1.1.1" {
		t.Fatalf("detail network = %+v", detail.Network)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/sandboxes", nil)
	listReq.Header.Set("X-API-KEY", "dev")
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var listed []listedSandboxResponse
	if err := json.NewDecoder(listRec.Body).Decode(&listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("listed len = %d", len(listed))
	}
	if listed[0].Metadata["team"] != "acme" || listed[0].Alias != "base" {
		t.Fatalf("listed item = %+v", listed[0])
	}
}

func TestCreateSandbox(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"templateID":"base","secure":false}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID == "" || resp.SandboxID != actors.lastActorID {
		t.Fatalf("sandboxID = %q, actor = %q", resp.SandboxID, actors.lastActorID)
	}
	if resp.Domain != "localhost" {
		t.Fatalf("domain = %q", resp.Domain)
	}
	if resp.EnvdVersion != "0.1.0" {
		t.Fatalf("envdVersion = %q", resp.EnvdVersion)
	}
	if resp.EnvdAccessToken != "" {
		t.Fatalf("envdAccessToken = %q, want empty", resp.EnvdAccessToken)
	}
	got, ok := st.records[resp.SandboxID]
	if !ok {
		t.Fatalf("sandbox %q not in store", resp.SandboxID)
	}
	if got.ActorID != resp.SandboxID || got.Template != "base" || got.Status != store.StatusRunning {
		t.Fatalf("stored sandbox = %+v", got)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at is zero")
	}
	if !got.ExpiresAt.Equal(now.Add(300 * time.Second)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, now.Add(300*time.Second))
	}
	if got.OnTimeout != store.OnTimeoutKill {
		t.Fatalf("on_timeout = %q, want %q", got.OnTimeout, store.OnTimeoutKill)
	}
}

func TestCreateSecureSandbox(t *testing.T) {
	t.Parallel()

	envdSrv := httptest.NewServer(envd.NewTestHandler(slog.Default()))
	t.Cleanup(envdSrv.Close)

	actors := &fakeActors{backendAddr: envdSrv.Listener.Addr().String()}
	st := newFakeStore()
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"templateID":"base","secure":true}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EnvdAccessToken == "" {
		t.Fatal("envdAccessToken is empty")
	}
	if resp.TrafficAccessToken == "" {
		t.Fatal("trafficAccessToken is empty")
	}

	got, ok := st.records[resp.SandboxID]
	if !ok {
		t.Fatalf("sandbox %q not in store", resp.SandboxID)
	}
	if !got.Secure {
		t.Fatal("stored sandbox not secure")
	}
	if got.EnvdAccessToken != resp.EnvdAccessToken {
		t.Fatalf("stored token = %q, response = %q", got.EnvdAccessToken, resp.EnvdAccessToken)
	}

	client := processv1connect.NewProcessClient(envdSrv.Client(), envdSrv.URL)
	listReq := connect.NewRequest(&processv1.ListRequest{})
	listReq.Header().Set("X-Access-Token", resp.EnvdAccessToken)
	if _, err := client.List(context.Background(), listReq); err != nil {
		t.Fatalf("envd list with configured token: %v", err)
	}
}

func TestGetSecureSandboxReturnsToken(t *testing.T) {
	t.Parallel()

	st := newFakeStore()
	st.records["sb-secure"] = store.Sandbox{
		SandboxID:       "sb-secure",
		ActorID:         "sb-secure",
		Template:        "base",
		CreatedAt:       time.Now().UTC(),
		Status:          store.StatusRunning,
		Secure:          true,
		EnvdAccessToken: "tok-123",
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-secure", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EnvdAccessToken != "tok-123" {
		t.Fatalf("envdAccessToken = %q", resp.EnvdAccessToken)
	}
}

func TestCreateSandboxWithTimeout(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"templateID":"base","secure":false,"timeout":60}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := st.records[resp.SandboxID]
	if !got.ExpiresAt.Equal(now.Add(60 * time.Second)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, now.Add(60*time.Second))
	}
}

func TestCreateSandboxLifecycleKill(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(testConfig(), actors, st, slog.Default())

	body := []byte(`{"templateID":"base","secure":false,"lifecycle":{"onTimeout":"kill"}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.records[resp.SandboxID].OnTimeout != store.OnTimeoutKill {
		t.Fatalf("on_timeout = %q, want kill", st.records[resp.SandboxID].OnTimeout)
	}
}

func TestCreateSandboxLifecyclePause(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(testConfig(), actors, st, slog.Default())

	body := []byte(`{"templateID":"base","secure":false,"lifecycle":{"onTimeout":"pause"}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := st.records[resp.SandboxID]
	if got.OnTimeout != store.OnTimeoutPause {
		t.Fatalf("on_timeout = %q, want pause", got.OnTimeout)
	}
	if got.AutoResume {
		t.Fatal("auto_resume = true, want false")
	}
}

func TestCreateSandboxLifecyclePauseWithAutoResume(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(testConfig(), actors, st, slog.Default())

	body := []byte(`{"templateID":"base","secure":false,"autoPause":true,"autoResume":{"enabled":true},"lifecycle":{"onTimeout":"pause"}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := st.records[resp.SandboxID]
	if got.OnTimeout != store.OnTimeoutPause {
		t.Fatalf("on_timeout = %q, want pause", got.OnTimeout)
	}
	if !got.AutoResume {
		t.Fatal("auto_resume = false, want true")
	}
}

func TestCreateSandboxAutoPause(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	srv := NewServer(testConfig(), actors, st, slog.Default())

	body := []byte(`{"templateID":"base","autoPause":true}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if st.records[resp.SandboxID].OnTimeout != store.OnTimeoutPause {
		t.Fatalf("on_timeout = %q, want pause", st.records[resp.SandboxID].OnTimeout)
	}
}

func TestCreateSandboxAutoResumeKillRejected(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	body := []byte(`{"templateID":"base","autoResume":{"enabled":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSandboxLifecyclePauseAutoPauseConflict(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	body := []byte(`{"templateID":"base","autoPause":false,"lifecycle":{"onTimeout":"pause"}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSandboxLifecycleUnknownRejected(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	body := []byte(`{"templateID":"base","lifecycle":{"onTimeout":"destroy"}}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSandboxInvalidTimeout(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"base","timeout":0}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestSetSandboxTimeout(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: now.Add(60 * time.Second),
		Status:    store.StatusRunning,
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now.Add(30 * time.Second) }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/timeout", bytes.NewReader([]byte(`{"timeout":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := now.Add(30 * time.Second).Add(120 * time.Second)
	if !st.records["sb-1"].ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", st.records["sb-1"].ExpiresAt, want)
	}
}

func TestSetSandboxTimeoutNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/missing/timeout", bytes.NewReader([]byte(`{"timeout":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestSetSandboxTimeoutInvalid(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/timeout", bytes.NewReader([]byte(`{"timeout":1}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRefreshSandbox(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: now.Add(60 * time.Second),
		Status:    store.StatusRunning,
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now.Add(30 * time.Second) }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/refreshes", bytes.NewReader([]byte(`{"duration":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", rec.Body.String())
	}
	want := now.Add(30 * time.Second).Add(120 * time.Second)
	if !st.records["sb-1"].ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", st.records["sb-1"].ExpiresAt, want)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var detail sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&detail); err != nil {
		t.Fatalf("decode GET: %v", err)
	}
	if detail.EndAt != want.UTC().Format(time.RFC3339) {
		t.Fatalf("endAt = %q, want %q", detail.EndAt, want.UTC().Format(time.RFC3339))
	}
}

func TestRefreshSandboxDefaultDuration(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: now.Add(60 * time.Second),
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/refreshes", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := now.Add(300 * time.Second)
	if !st.records["sb-1"].ExpiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", st.records["sb-1"].ExpiresAt, want)
	}
}

func TestRefreshSandboxNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/missing/refreshes", bytes.NewReader([]byte(`{"duration":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestRefreshSandboxInvalidDuration(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	for _, body := range []string{`{"duration":0}`, `{"duration":3601}`} {
		req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/refreshes", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-KEY", "dev")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestListSandboxMetrics(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics?sandbox_ids=sb-1,sb-2", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxesWithMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Sandboxes) != 2 {
		t.Fatalf("sandboxes len = %d, want 2", len(resp.Sandboxes))
	}
	for _, id := range []string{"sb-1", "sb-2"} {
		m, ok := resp.Sandboxes[id]
		if !ok {
			t.Fatalf("missing sandbox %q", id)
		}
		if m.TimestampUnix != now.Unix() || m.CPUCount != defaultCPUCount || m.CPUUsedPct != 0 {
			t.Fatalf("metric for %q = %+v", id, m)
		}
	}
}

func TestListSandboxMetricsMissingIDs(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListSandboxMetricsFromEnvd(t *testing.T) {
	t.Parallel()
	backend := testEnvdBackend(t)
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: backend}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/metrics?sandbox_ids=sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxesWithMetricsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	m := resp.Sandboxes["sb-1"]
	if m.MemUsed == 0 || m.MemTotal == 0 {
		t.Fatalf("metric = %+v", m)
	}
}

func TestGetSandboxMetricsFromEnvd(t *testing.T) {
	t.Parallel()
	backend := testEnvdBackend(t)
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: backend}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/metrics", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var samples []sandboxMetricResponse
	if err := json.NewDecoder(rec.Body).Decode(&samples); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(samples) != 1 || samples[0].MemUsed == 0 {
		t.Fatalf("samples = %+v", samples)
	}
}

func TestGetSandboxMetrics(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/metrics?start=0&end=100", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var samples []sandboxMetricResponse
	if err := json.NewDecoder(rec.Body).Decode(&samples); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(samples) != 0 {
		t.Fatalf("samples = %+v, want empty", samples)
	}
}

func TestGetSandboxMetricsNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/missing/metrics", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetSandboxMetricsInvalidQuery(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/metrics?start=bad", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetSandboxLogsFromEnvd(t *testing.T) {
	t.Parallel()
	backend := testEnvdBackend(t)
	seedEnvdLog(t, backend, "hello")

	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: backend}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs?start=0&limit=100", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxLogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Line != "hello" {
		t.Fatalf("logs = %+v, want hello line", resp.Logs)
	}
	if len(resp.LogEntries) != 1 || resp.LogEntries[0].Message != "hello" {
		t.Fatalf("logEntries = %+v, want hello entry", resp.LogEntries)
	}
}

func TestGetSandboxLogsEnvdUnreachable(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{backendErr: fmt.Errorf("no worker")}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp sandboxLogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 0 || len(resp.LogEntries) != 0 {
		t.Fatalf("expected empty logs, got %+v", resp)
	}
}

func TestGetSandboxLogs(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs?start=0&limit=100", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxLogsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Logs == nil || resp.LogEntries == nil {
		t.Fatalf("nil slices in response: %+v", resp)
	}
	if len(resp.Logs) != 0 || len(resp.LogEntries) != 0 {
		t.Fatalf("expected empty logs, got %+v", resp)
	}
}

func TestGetSandboxLogsNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/missing/logs", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetSandboxLogsInvalidQuery(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1/logs?start=bad", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetSandboxLogsV2FromEnvd(t *testing.T) {
	t.Parallel()
	backend := testEnvdBackend(t)
	seedEnvdLog(t, backend, "hello")

	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: backend}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/sb-1/logs?cursor=0&limit=50&direction=forward&level=info", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxLogsV2Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Logs) != 1 || resp.Logs[0].Message != "hello" {
		t.Fatalf("logs = %+v, want hello entry", resp.Logs)
	}
	if resp.Logs[0].Level != "info" || resp.Logs[0].Fields["stream"] != "stdout" {
		t.Fatalf("entry = %+v", resp.Logs[0])
	}
}

func TestGetSandboxLogsV2(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/sb-1/logs?cursor=0&limit=50&direction=forward&level=warn&search=test", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxLogsV2Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Logs == nil {
		t.Fatalf("nil logs slice in response: %+v", resp)
	}
	if len(resp.Logs) != 0 {
		t.Fatalf("expected empty logs, got %+v", resp)
	}
}

func TestGetSandboxLogsV2NotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/missing/logs", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetSandboxLogsV2InvalidQuery(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes/sb-1/logs?limit=1001", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateSandboxUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"base"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestDeleteSandbox(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{}
	st := newFakeStore()
	st.records["abc-123"] = store.Sandbox{SandboxID: "abc-123", ActorID: "abc-123"}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/abc-123", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastDeletedID != "abc-123" {
		t.Fatalf("deleted id = %q, want abc-123", actors.lastDeletedID)
	}
	if _, ok := st.records["abc-123"]; ok {
		t.Fatal("sandbox still in store after delete")
	}
}

func TestDeleteSandboxNotFound(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{deleteErr: runtimeapi.ErrNotFound}
	srv := NewServer(testConfig(), actors, newFakeStore(), slog.Default())

	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDeleteSandboxUnauthorized(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodDelete, "/sandboxes/abc-123", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestCreateSandboxUnsupportedTemplate(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes", bytes.NewReader([]byte(`{"templateID":"other"}`)))
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetSandbox(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(120 * time.Second)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID != "sb-1" || resp.State != "running" || resp.TemplateID != "base" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.EndAt != expiresAt.Format(time.RFC3339) {
		t.Fatalf("endAt = %q, want %q", resp.EndAt, expiresAt.Format(time.RFC3339))
	}
	if resp.Lifecycle.OnTimeout != store.OnTimeoutKill || resp.Lifecycle.AutoResume {
		t.Fatalf("lifecycle = %+v", resp.Lifecycle)
	}
}

func TestGetSandboxPaused(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(120 * time.Second)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID:  "sb-1",
		ActorID:    "sb-1",
		Template:   "base",
		CreatedAt:  createdAt,
		ExpiresAt:  expiresAt,
		OnTimeout:  store.OnTimeoutPause,
		AutoResume: true,
		Status:     store.StatusPaused,
	}
	actors := &fakeActors{
		actorStatuses: map[string]runtimeapipb.Actor_Status{
			"sb-1": runtimeapipb.Actor_STATUS_SUSPENDED,
		},
	}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.State != "paused" {
		t.Fatalf("state = %q, want paused", resp.State)
	}
	if resp.Lifecycle.OnTimeout != store.OnTimeoutPause || !resp.Lifecycle.AutoResume {
		t.Fatalf("lifecycle = %+v", resp.Lifecycle)
	}
}

func TestPauseSandbox(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/pause", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastSuspended != "sb-1" {
		t.Fatalf("suspended = %q", actors.lastSuspended)
	}
	if st.records["sb-1"].Status != store.StatusPaused {
		t.Fatalf("status = %q, want paused", st.records["sb-1"].Status)
	}
}

func TestPauseSandboxNotFound(t *testing.T) {
	t.Parallel()
	actors := &fakeActors{suspendErr: runtimeapi.ErrNotFound}
	srv := NewServer(testConfig(), actors, newFakeStore(), slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/missing/pause", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestResumeSandbox(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID:  "sb-1",
		ActorID:    "sb-1",
		Template:   "base",
		CreatedAt:  now,
		OnTimeout:  store.OnTimeoutPause,
		AutoResume: true,
		Status:     store.StatusPaused,
	}
	actors := &fakeActors{backendAddr: testEnvdBackend(t)}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	body := []byte(`{"timeout":60,"autoPause":true}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/resume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID != "sb-1" || resp.TemplateID != "base" || resp.Domain != "localhost" {
		t.Fatalf("resp = %+v", resp)
	}
	if resp.ClientID != "actordock" || resp.EnvdVersion != "0.1.0" {
		t.Fatalf("resp = %+v", resp)
	}
	if actors.lastResumed != "sb-1" {
		t.Fatalf("resumed = %q", actors.lastResumed)
	}
	got := st.records["sb-1"]
	if got.Status != store.StatusRunning {
		t.Fatalf("status = %q, want running", got.Status)
	}
	if !got.ExpiresAt.Equal(now.Add(60 * time.Second)) {
		t.Fatalf("expires_at = %v", got.ExpiresAt)
	}
	if got.OnTimeout != store.OnTimeoutPause || !got.AutoResume {
		t.Fatalf("lifecycle fields = %+v", got)
	}
}

func TestConnectSandboxRunningExtendsTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: now.Add(5 * time.Minute),
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{backendAddr: testEnvdBackend(t)}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/connect", bytes.NewReader([]byte(`{"timeout":600}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastResumed != "" {
		t.Fatalf("ResumeSandbox called for running sandbox")
	}
	if !st.records["sb-1"].ExpiresAt.Equal(now.Add(600 * time.Second)) {
		t.Fatalf("expires_at = %v, want extended TTL", st.records["sb-1"].ExpiresAt)
	}
	var resp sandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SandboxID != "sb-1" || resp.Domain != "localhost" || resp.TemplateID != "base" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestConnectSandboxPausedResumes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		Status:    store.StatusPaused,
	}
	actors := &fakeActors{backendAddr: testEnvdBackend(t)}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/connect", bytes.NewReader([]byte(`{"timeout":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if actors.lastResumed != "sb-1" {
		t.Fatalf("resumed = %q, want sb-1", actors.lastResumed)
	}
	if st.records["sb-1"].Status != store.StatusRunning {
		t.Fatalf("status = %q, want running", st.records["sb-1"].Status)
	}
	if !st.records["sb-1"].ExpiresAt.Equal(now.Add(120 * time.Second)) {
		t.Fatalf("expires_at = %v", st.records["sb-1"].ExpiresAt)
	}
}

func TestConnectSandboxNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/missing/connect", bytes.NewReader([]byte(`{"timeout":60}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestConnectSandboxInvalidTimeout(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: testEnvdBackend(t)}, st, slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/connect", bytes.NewReader([]byte(`{"timeout":1}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestConnectSandboxDoesNotShortenTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	longExpiry := now.Add(300 * time.Second)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: now,
		ExpiresAt: longExpiry,
		Status:    store.StatusRunning,
	}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: testEnvdBackend(t)}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/connect", bytes.NewReader([]byte(`{"timeout":30}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !st.records["sb-1"].ExpiresAt.Equal(longExpiry) {
		t.Fatalf("expires_at = %v, want %v", st.records["sb-1"].ExpiresAt, longExpiry)
	}
}

func TestResolveConnectExpiresAt(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	current := now.Add(300 * time.Second)

	got := resolveConnectExpiresAt(now, current, 30, false)
	if !got.Equal(current) {
		t.Fatalf("running shorten: got %v want %v", got, current)
	}

	got = resolveConnectExpiresAt(now, current, 600, false)
	want := now.Add(600 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("running extend: got %v want %v", got, want)
	}

	got = resolveConnectExpiresAt(now, current, 30, true)
	want = now.Add(30 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("paused: got %v want %v", got, want)
	}
}

func TestResumeSandboxDefaultTimeout(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		Status:    store.StatusPaused,
	}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: testEnvdBackend(t)}, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/resume", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !st.records["sb-1"].ExpiresAt.Equal(now.Add(15 * time.Second)) {
		t.Fatalf("expires_at = %v, want +15s", st.records["sb-1"].ExpiresAt)
	}
}

func TestResumeSandboxAutoPauseKill(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID:  "sb-1",
		ActorID:    "sb-1",
		Template:   "base",
		OnTimeout:  store.OnTimeoutPause,
		AutoResume: true,
		Status:     store.StatusPaused,
	}
	srv := NewServer(testConfig(), &fakeActors{backendAddr: testEnvdBackend(t)}, st, slog.Default())

	body := []byte(`{"timeout":60,"autoPause":false}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/resume", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := st.records["sb-1"]
	if got.OnTimeout != store.OnTimeoutKill || got.AutoResume {
		t.Fatalf("on_timeout = %q auto_resume = %v", got.OnTimeout, got.AutoResume)
	}
}

func TestGetSandboxSyncsAfterAutoResume(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	createdAt := now.Add(-time.Hour)
	expiresAt := now.Add(-time.Minute)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		OnTimeout: store.OnTimeoutPause,
		Status:    store.StatusPaused,
	}
	actors := &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	srv.nowFunc = func() time.Time { return now }

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	got := st.records["sb-1"]
	if got.Status != store.StatusRunning {
		t.Fatalf("status = %q, want running", got.Status)
	}
	if !got.ExpiresAt.Equal(now.Add(300 * time.Second)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, now.Add(300*time.Second))
	}
}

func TestGetSandboxResuming(t *testing.T) {
	t.Parallel()
	createdAt := time.Now().UTC()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{
		actorStatuses: map[string]runtimeapipb.Actor_Status{
			"sb-1": runtimeapipb.Actor_STATUS_RESUMING,
		},
	}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.State != "running" {
		t.Fatalf("state = %q, want running", resp.State)
	}
	if st.records["sb-1"].Status != store.StatusPending {
		t.Fatalf("stored status = %q, want pending", st.records["sb-1"].Status)
	}
}

func TestGetSandboxEndAtAfterSetTimeout(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		ExpiresAt: createdAt.Add(60 * time.Second),
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())
	extendAt := createdAt.Add(30 * time.Second)
	srv.nowFunc = func() time.Time { return extendAt }

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/timeout", bytes.NewReader([]byte(`{"timeout":120}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("set timeout status = %d", rec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, body = %s", getRec.Code, getRec.Body.String())
	}
	var resp sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantEndAt := extendAt.Add(120 * time.Second).Format(time.RFC3339)
	if resp.EndAt != wantEndAt {
		t.Fatalf("endAt = %q, want %q", resp.EndAt, wantEndAt)
	}
}

func TestGetSandboxNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/sandboxes/missing", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestGetSandboxActorGone(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Template: "base", CreatedAt: time.Now()}
	actors := &fakeActors{getErr: runtimeapi.ErrNotFound}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if _, ok := st.records["sb-1"]; ok {
		t.Fatal("stale sandbox not purged from store")
	}
}

func TestListSandboxes(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.records["a"] = store.Sandbox{
		SandboxID: "a", ActorID: "a", Template: "base",
		CreatedAt: now, ExpiresAt: now.Add(60 * time.Second), Status: store.StatusRunning,
	}
	st.records["b"] = store.Sandbox{
		SandboxID: "b", ActorID: "b", Template: "base",
		CreatedAt: now, ExpiresAt: now.Add(90 * time.Second), Status: store.StatusRunning,
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/sandboxes", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []listedSandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 2 {
		t.Fatalf("len = %d, want 2", len(resp))
	}
	endAts := map[string]string{}
	for _, item := range resp {
		endAts[item.SandboxID] = item.EndAt
	}
	if endAts["a"] != now.Add(60*time.Second).Format(time.RFC3339) {
		t.Fatalf("endAt for a = %q", endAts["a"])
	}
	if endAts["b"] != now.Add(90*time.Second).Format(time.RFC3339) {
		t.Fatalf("endAt for b = %q", endAts["b"])
	}
}

func TestListSandboxesV2(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC()
	st := newFakeStore()
	st.records["a"] = store.Sandbox{SandboxID: "a", ActorID: "a", Template: "base", CreatedAt: now, Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v2/sandboxes", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []listedSandboxResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].SandboxID != "a" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestPutSandboxNetworkRoundTrip(t *testing.T) {
	t.Parallel()
	createdAt := time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC)
	expiresAt := createdAt.Add(120 * time.Second)
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		CreatedAt: createdAt,
		ExpiresAt: expiresAt,
		Status:    store.StatusRunning,
	}
	srv := NewServer(testConfig(), &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}, st, slog.Default())

	putBody := []byte(`{
		"allowOut":["1.1.1.1"],
		"denyOut":["8.8.8.8"],
		"allow_internet_access":false,
		"rules":{"api.example.com":[{"transform":{"headers":{"X-Test":"1"}}}]}
	}`)
	putReq := httptest.NewRequest(http.MethodPut, "/sandboxes/sb-1/network", bytes.NewReader(putBody))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-API-KEY", "dev")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var resp sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AllowInternetAccess == nil || *resp.AllowInternetAccess {
		t.Fatalf("allowInternetAccess = %v, want false", resp.AllowInternetAccess)
	}
	if resp.Network == nil {
		t.Fatal("network is nil")
	}
	if len(resp.Network.AllowOut) != 1 || resp.Network.AllowOut[0] != "1.1.1.1" {
		t.Fatalf("allowOut = %v", resp.Network.AllowOut)
	}
	if len(resp.Network.DenyOut) != 1 || resp.Network.DenyOut[0] != "8.8.8.8" {
		t.Fatalf("denyOut = %v", resp.Network.DenyOut)
	}
	rules := resp.Network.Rules["api.example.com"]
	if len(rules) != 1 || rules[0].Transform.Headers["X-Test"] != "1" {
		t.Fatalf("rules = %+v", resp.Network.Rules)
	}
}

func TestPutSandboxNetworkClearsExistingRules(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		Status:    store.StatusRunning,
		Network: &store.NetworkConfig{
			AllowOut: []string{"1.1.1.1"},
			DenyOut:  []string{"8.8.8.8"},
		},
		AllowInternetAccess: boolPtr(false),
	}
	srv := NewServer(testConfig(), &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}, st, slog.Default())

	putReq := httptest.NewRequest(http.MethodPut, "/sandboxes/sb-1/network", bytes.NewReader([]byte(`{}`)))
	putReq.Header.Set("Content-Type", "application/json")
	putReq.Header.Set("X-API-KEY", "dev")
	putRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRec, putReq)
	if putRec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, body = %s", putRec.Code, putRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/sandboxes/sb-1", nil)
	getReq.Header.Set("X-API-KEY", "dev")
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", getRec.Code, getRec.Body.String())
	}

	var resp sandboxDetailResponse
	if err := json.NewDecoder(getRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Network != nil {
		t.Fatalf("network = %+v, want nil", resp.Network)
	}
	if resp.AllowInternetAccess != nil {
		t.Fatalf("allowInternetAccess = %v, want null", resp.AllowInternetAccess)
	}
}

func TestPutSandboxNetworkNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPut, "/sandboxes/missing/network", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestPutSandboxNetworkInvalidBody(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}, st, slog.Default())

	cases := []struct {
		name string
		body string
	}{
		{name: "invalid json", body: `{`},
		{name: "allowOut type", body: `{"allowOut":"8.8.8.8"}`},
		{name: "deny domain", body: `{"denyOut":["example.com"]}`},
		{name: "missing transform", body: `{"rules":{"example.com":[{}]}}`},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodPut, "/sandboxes/sb-1/network", bytes.NewReader([]byte(tc.body)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-KEY", "dev")
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCreateSandboxSnapshot(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{
		SandboxID: "sb-1",
		ActorID:   "sb-1",
		Template:  "base",
		Status:    store.StatusRunning,
	}
	actors := &fakeActors{defaultStatus: runtimeapipb.Actor_STATUS_RUNNING}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/snapshots", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp snapshotInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.SnapshotID == "" || !strings.HasSuffix(resp.SnapshotID, ":default") {
		t.Fatalf("snapshotID = %q", resp.SnapshotID)
	}
	if len(resp.Names) != 1 || resp.Names[0] != resp.SnapshotID {
		t.Fatalf("names = %v", resp.Names)
	}
	if actors.lastSnapshotActor != "sb-1" {
		t.Fatalf("snapshot actor = %q", actors.lastSnapshotActor)
	}

	snap, err := st.GetSnapshot(context.Background(), resp.SnapshotID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.SandboxID != "sb-1" || snap.ActorID != "sb-1" || snap.SnapshotURI == "" {
		t.Fatalf("stored snapshot = %+v", snap)
	}
	if st.records["sb-1"].Status != store.StatusPaused {
		t.Fatalf("sandbox status = %q, want paused", st.records["sb-1"].Status)
	}
}

func TestCreateSandboxSnapshotNamed(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	body := []byte(`{"name":"my-snap"}`)
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/snapshots", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp snapshotInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	wantID := "actordock/my-snap:default"
	if resp.SnapshotID != wantID {
		t.Fatalf("snapshotID = %q, want %q", resp.SnapshotID, wantID)
	}
}

func TestCreateSandboxSnapshotNotFound(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/sandboxes/missing/snapshots", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestCreateSandboxSnapshotInvalidState(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Status: store.StatusRunning}
	actors := &fakeActors{createSnapshotErr: runtimeapi.ErrInvalidState}
	srv := NewServer(testConfig(), actors, st, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/snapshots", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestListSnapshotsEmpty(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/snapshots", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp []snapshotInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 0 {
		t.Fatalf("resp = %+v, want empty", resp)
	}
}

func TestListSnapshotsAfterCreate(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.records["sb-1"] = store.Sandbox{SandboxID: "sb-1", ActorID: "sb-1", Status: store.StatusRunning}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	createReq := httptest.NewRequest(http.MethodPost, "/sandboxes/sb-1/snapshots", bytes.NewReader([]byte(`{}`)))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.Header.Set("X-API-KEY", "dev")
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/snapshots", nil)
	listReq.Header.Set("X-API-KEY", "dev")
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}

	var resp []snapshotInfoResponse
	if err := json.NewDecoder(listRec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].SnapshotID == "" || len(resp[0].Names) != 1 {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestListSnapshotsFilterBySandboxID(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.snapshots["snap-a"] = store.Snapshot{
		SnapshotID: "snap-a:default",
		Names:      []string{"snap-a:default"},
		SandboxID:  "sb-1",
		CreatedAt:  now,
	}
	st.snapshots["snap-b"] = store.Snapshot{
		SnapshotID: "snap-b:default",
		Names:      []string{"snap-b:default"},
		SandboxID:  "sb-2",
		CreatedAt:  now.Add(time.Minute),
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/snapshots?sandboxID=sb-1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp []snapshotInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 || resp[0].SnapshotID != "snap-a:default" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestListSnapshotsPagination(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	st := newFakeStore()
	st.snapshots["snap-old:default"] = store.Snapshot{
		SnapshotID: "snap-old:default",
		Names:      []string{"snap-old:default"},
		SandboxID:  "sb-1",
		CreatedAt:  now,
	}
	st.snapshots["snap-new:default"] = store.Snapshot{
		SnapshotID: "snap-new:default",
		Names:      []string{"snap-new:default"},
		SandboxID:  "sb-1",
		CreatedAt:  now.Add(time.Minute),
	}
	srv := NewServer(testConfig(), &fakeActors{}, st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/snapshots?limit=1", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	next := rec.Header().Get("X-Next-Token")
	if next == "" {
		t.Fatal("expected X-Next-Token header")
	}

	var page1 []snapshotInfoResponse
	if err := json.NewDecoder(rec.Body).Decode(&page1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page1) != 1 || page1[0].SnapshotID != "snap-new:default" {
		t.Fatalf("page1 = %+v", page1)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/snapshots?limit=1&nextToken="+next, nil)
	req2.Header.Set("X-API-KEY", "dev")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	if rec2.Header().Get("X-Next-Token") != "" {
		t.Fatalf("unexpected next token on last page")
	}

	var page2 []snapshotInfoResponse
	if err := json.NewDecoder(rec2.Body).Decode(&page2); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(page2) != 1 || page2[0].SnapshotID != "snap-old:default" {
		t.Fatalf("page2 = %+v", page2)
	}
}

func TestListSnapshotsInvalidLimit(t *testing.T) {
	t.Parallel()
	srv := NewServer(testConfig(), &fakeActors{}, newFakeStore(), slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/snapshots?limit=0", nil)
	req.Header.Set("X-API-KEY", "dev")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func boolPtr(v bool) *bool { return &v }

func testConfig() config.Platform {
	return config.Platform{
		Server: config.Server{
			ListenAddr: ":8080",
			LogLevel:   "info",
		},
		APIKey:                "dev",
		Domain:                "localhost",
		TemplateNamespace:     "actordock",
		TemplateName:          "base",
		EnvdVersion:           "0.1.0",
		EnvdPort:              80,
		ClientID:              "actordock",
		DefaultSandboxTimeout: 300,
		VolumeRoot:            "/var/lib/actordock/volumes",
		OfficialBaseTemplates: []string{"base", "python"},
	}
}

func testEnvdBackend(t *testing.T) string {
	t.Helper()
	return envd.StartStubTestBackend(t)
}

func seedEnvdLog(t *testing.T, backend, message string) {
	t.Helper()
	client := processv1connect.NewProcessClient(envd.NewHTTPClient(), "http://"+backend)
	stream, err := client.Start(context.Background(), connect.NewRequest(&processv1.StartRequest{
		Process: &processv1.ProcessConfig{
			Cmd:  "/bin/sh",
			Args: []string{"-c", "echo " + message},
		},
	}))
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	for stream.Receive() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream: %v", err)
	}
}
