// Copyright 2026 The Actordock Authors.
// SPDX-License-Identifier: Apache-2.0

package policy

// tryStickyResume keeps a paused/suspended sandbox on its last Worker when that slot is free.
func tryStickyResume(req ResumeRequest, reason string) (PlaceResult, bool) {
	if req.Sandbox.WorkerID == "" {
		return PlaceResult{}, false
	}
	for _, w := range req.Workers {
		if w.ID != req.Sandbox.WorkerID {
			continue
		}
		if !workerEligibleForPlace(w, req.WorkerSignals) {
			return PlaceResult{}, false
		}
		if workerHostingCheckpoint(w.ID, req.Running, req.SandboxSignals) {
			return PlaceResult{}, false
		}
		return PlaceResult{WorkerID: w.ID, Reason: reason}, true
	}
	return PlaceResult{}, false
}

func placeFromResume(req ResumeRequest) PlaceRequest {
	return PlaceRequest{
		SandboxID:     req.Sandbox.ID,
		Workers:       req.Workers,
		Running:       req.Running,
		SandboxSignals: req.SandboxSignals,
		WorkerSignals: req.WorkerSignals,
	}
}
