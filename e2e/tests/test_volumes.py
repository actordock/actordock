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

"""E2E volume CRUD + volumeMounts round-trip (v0.0.9)."""

from __future__ import annotations

import os
import uuid

import httpx
from e2b import Sandbox, Volume


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {
        "X-API-KEY": os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }


def test_volume_crud_via_sdk() -> None:
    name = f"e2e-vol-{uuid.uuid4().hex[:8]}"
    volume = Volume.create(name)
    try:
        assert volume.name == name
        assert volume.volume_id

        listed = Volume.list()
        assert name in {item.name for item in listed}

        fetched = Volume.get_info(volume.volume_id)
        assert fetched.name == name
        assert fetched.volume_id == volume.volume_id
    finally:
        Volume.destroy(volume.volume_id)


def test_volume_invalid_name_400() -> None:
    resp = httpx.post(
        f"{_api_url()}/volumes",
        headers=_api_headers(),
        json={"name": "bad name"},
        timeout=30.0,
    )
    assert resp.status_code == 400, resp.text


def test_create_sandbox_unknown_volume_400() -> None:
    resp = httpx.post(
        f"{_api_url()}/sandboxes",
        headers=_api_headers(),
        json={
            "templateID": "base",
            "secure": False,
            "volumeMounts": [{"name": "missing-volume", "path": "/mnt/data"}],
        },
        timeout=120.0,
    )
    assert resp.status_code == 400, resp.text


def test_volume_mounts_round_trip() -> None:
    name = f"e2e-mount-{uuid.uuid4().hex[:8]}"
    volume = Volume.create(name)
    sbx: Sandbox | None = None
    try:
        sbx = Sandbox.create(
            template="base",
            secure=False,
            volume_mounts={"/mnt/data": volume},
            timeout=120,
        )
        resp = httpx.get(
            f"{_api_url()}/sandboxes/{sbx.sandbox_id}",
            headers=_api_headers(),
            timeout=30.0,
        )
        assert resp.status_code == 200, resp.text
        mounts = resp.json().get("volumeMounts") or []
        assert len(mounts) == 1
        assert mounts[0]["name"] == name
        assert mounts[0]["path"] == "/mnt/data"
    finally:
        if sbx is not None:
            sbx.kill()
        Volume.destroy(volume.volume_id)
