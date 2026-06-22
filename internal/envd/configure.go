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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ConfigureAccessToken programs envd to require X-Access-Token on subsequent requests.
func ConfigureAccessToken(ctx context.Context, baseURL, token string) error {
	if token == "" {
		return nil
	}

	body, err := json.Marshal(initRequest{AccessToken: token})
	if err != nil {
		return fmt.Errorf("marshal init request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, normalizeBaseURL(baseURL)+"/init", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new init request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := newH2CHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("post init: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("post init: status %d", resp.StatusCode)
	}
	return nil
}
