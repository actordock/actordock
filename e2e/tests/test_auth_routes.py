# Copyright 2026 The Actordock Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

"""E2E auth routes smoke (v0.1.0 WP4)."""

from __future__ import annotations

import os

import httpx


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def test_list_api_keys_includes_default() -> None:
    resp = httpx.get(
        f"{_api_url()}/api-keys",
        headers=_api_headers(),
        timeout=30.0,
    )
    resp.raise_for_status()
    keys = resp.json()
    assert isinstance(keys, list)
    assert len(keys) >= 1
    first = keys[0]
    assert first["name"] == "default"
    assert first["id"]
    assert first["mask"]["valueLength"] >= 1
    assert first["createdAt"]
