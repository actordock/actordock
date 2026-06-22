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

"""E2E template write APIs (v0.1.0 WP5 + WP7)."""

from __future__ import annotations

import uuid

import httpx

from support.api import api_headers, api_url

TEMPLATE_FIELDS = frozenset(
    {
        "templateID",
        "buildID",
        "cpuCount",
        "memoryMB",
        "diskSizeMB",
        "public",
        "aliases",
        "names",
        "createdAt",
        "updatedAt",
        "createdBy",
        "lastSpawnedAt",
        "spawnCount",
        "buildCount",
        "envdVersion",
        "buildStatus",
    }
)


def test_post_template_metadata_only() -> None:
    alias = f"e2e-{uuid.uuid4().hex[:8]}"
    resp = httpx.post(
        f"{api_url()}/templates",
        headers=api_headers(),
        json={
            "alias": alias,
            "dockerfile": "FROM scratch",
            "cpuCount": 2,
            "memoryMB": 512,
        },
        timeout=30.0,
    )
    assert resp.status_code == 201, resp.text
    body = resp.json()
    assert TEMPLATE_FIELDS <= set(body)
    assert body["aliases"] == [alias]
    assert body["buildStatus"] == "ready"
    assert body["cpuCount"] == 2
    assert body["memoryMB"] == 512

    get_resp = httpx.get(
        f"{api_url()}/templates/{body['templateID']}",
        headers=api_headers(),
        timeout=30.0,
    )
    assert get_resp.status_code == 200, get_resp.text
    assert get_resp.json()["templateID"] == body["templateID"]


def test_patch_template_public() -> None:
    alias = f"e2e-patch-{uuid.uuid4().hex[:8]}"
    create = httpx.post(
        f"{api_url()}/templates",
        headers=api_headers(),
        json={"alias": alias, "dockerfile": "FROM scratch"},
        timeout=30.0,
    )
    assert create.status_code == 201, create.text
    template_id = create.json()["templateID"]
    assert create.json()["public"] is False

    patch = httpx.patch(
        f"{api_url()}/templates/{template_id}",
        headers=api_headers(),
        json={"public": True},
        timeout=30.0,
    )
    assert patch.status_code == 200, patch.text
    updated = patch.json()
    assert updated["names"] == create.json()["names"]

    get_resp = httpx.get(
        f"{api_url()}/templates/{template_id}",
        headers=api_headers(),
        timeout=30.0,
    )
    assert get_resp.status_code == 200, get_resp.text
    assert get_resp.json()["public"] is True
