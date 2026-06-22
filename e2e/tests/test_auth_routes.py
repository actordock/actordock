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

"""E2E auth routes (v0.1.0 WP4 + WP7)."""

from __future__ import annotations

import httpx

from support.api import api_headers, api_url

API_KEY_FIELDS = frozenset(
    {"id", "name", "mask", "createdAt", "createdBy", "lastUsed"}
)
CREATED_API_KEY_FIELDS = API_KEY_FIELDS | {"key"}
MASK_FIELDS = frozenset(
    {"prefix", "valueLength", "maskedValuePrefix", "maskedValueSuffix"}
)
CREATED_ACCESS_TOKEN_FIELDS = frozenset(
    {"id", "name", "token", "mask", "createdAt"}
)


def test_list_api_keys_includes_default() -> None:
    resp = httpx.get(
        f"{api_url()}/api-keys",
        headers=api_headers(),
        timeout=30.0,
    )
    resp.raise_for_status()
    keys = resp.json()
    assert isinstance(keys, list)
    assert len(keys) >= 1
    first = keys[0]
    assert API_KEY_FIELDS <= set(first)
    assert first["name"] == "default"
    assert MASK_FIELDS <= set(first["mask"])


def test_create_api_key_full_schema_and_auth() -> None:
    create = httpx.post(
        f"{api_url()}/api-keys",
        headers=api_headers(),
        json={"name": "e2e-bot"},
        timeout=30.0,
    )
    assert create.status_code == 201, create.text
    body = create.json()
    assert CREATED_API_KEY_FIELDS <= set(body)
    assert MASK_FIELDS <= set(body["mask"])
    assert body["key"].startswith("adk_")
    assert body["mask"]["valueLength"] == len(body["key"])

    list_resp = httpx.get(
        f"{api_url()}/api-keys",
        headers=api_headers(api_key=body["key"]),
        timeout=30.0,
    )
    assert list_resp.status_code == 200, list_resp.text
    names = {item["name"] for item in list_resp.json()}
    assert "e2e-bot" in names
    assert "default" in names


def test_create_and_delete_access_token() -> None:
    create = httpx.post(
        f"{api_url()}/access-tokens",
        headers=api_headers(),
        json={"name": "e2e-dashboard"},
        timeout=30.0,
    )
    assert create.status_code == 201, create.text
    body = create.json()
    assert CREATED_ACCESS_TOKEN_FIELDS <= set(body)
    assert body["token"].startswith("adt_")
    assert MASK_FIELDS <= set(body["mask"])

    delete = httpx.delete(
        f"{api_url()}/access-tokens/{body['id']}",
        headers=api_headers(),
        timeout=30.0,
    )
    assert delete.status_code == 204, delete.text

    again = httpx.delete(
        f"{api_url()}/access-tokens/{body['id']}",
        headers=api_headers(),
        timeout=30.0,
    )
    assert again.status_code == 404, again.text


def test_create_api_key_requires_auth() -> None:
    resp = httpx.post(
        f"{api_url()}/api-keys",
        json={"name": "no-auth"},
        timeout=30.0,
    )
    assert resp.status_code == 401, resp.text
