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

"""E2E sandbox OpenAPI field parity (v0.1.0 WP3 + WP7)."""

from __future__ import annotations

import httpx

from support.api import api_headers, api_url
from support.parity import (
    LISTED_SANDBOX_FIELDS,
    LISTED_SANDBOX_OPTIONAL_FIELDS,
    LISTED_SANDBOX_OMIT_EMPTY_FIELDS,
    SANDBOX_DETAIL_FIELDS,
    SANDBOX_DETAIL_OPTIONAL_FIELDS,
    SANDBOX_DETAIL_OMIT_EMPTY_FIELDS,
)


def _create_body() -> dict:
    return {
        "templateID": "base",
        "secure": False,
        "timeout": 120,
        "metadata": {"team": "e2e", "run": "parity"},
        "envVars": {"E2E_FLAG": "1"},
        "mcp": {},
        "network": {"allowPublicTraffic": True},
        "allow_internet_access": True,
    }


def test_create_get_list_sandbox_fields() -> None:
    create = httpx.post(
        f"{api_url()}/sandboxes",
        headers=api_headers(),
        json=_create_body(),
        timeout=60.0,
    )
    assert create.status_code == 201, create.text
    created = create.json()
    sandbox_id = created["sandboxID"]
    try:
        for key in ("clientID", "envdVersion", "sandboxID", "templateID", "domain"):
            assert created[key], f"create.{key}"

        detail_resp = httpx.get(
            f"{api_url()}/sandboxes/{sandbox_id}",
            headers=api_headers(),
            timeout=30.0,
        )
        assert detail_resp.status_code == 200, detail_resp.text
        detail = detail_resp.json()
        assert SANDBOX_DETAIL_FIELDS <= set(detail)
        for key in SANDBOX_DETAIL_OPTIONAL_FIELDS:
            assert key in detail, f"missing detail.{key}"
        for key in SANDBOX_DETAIL_OMIT_EMPTY_FIELDS:
            assert key not in detail, f"unexpected empty detail.{key}"
        assert detail["metadata"] == _create_body()["metadata"]
        assert detail["allowInternetAccess"] is True
        assert detail["lifecycle"]["onTimeout"] in ("kill", "pause")
        assert isinstance(detail["lifecycle"]["autoResume"], bool)

        listed = httpx.get(
            f"{api_url()}/sandboxes",
            headers=api_headers(),
            timeout=30.0,
        )
        assert listed.status_code == 200, listed.text
        items = listed.json()
        match = next((item for item in items if item["sandboxID"] == sandbox_id), None)
        assert match is not None
        assert LISTED_SANDBOX_FIELDS <= set(match)
        for key in LISTED_SANDBOX_OPTIONAL_FIELDS:
            assert key in match, f"missing listed.{key}"
        for key in LISTED_SANDBOX_OMIT_EMPTY_FIELDS:
            assert key not in match, f"unexpected empty listed.{key}"
        assert match["metadata"] == _create_body()["metadata"]

        listed_v2 = httpx.get(
            f"{api_url()}/v2/sandboxes",
            headers=api_headers(),
            timeout=30.0,
        )
        assert listed_v2.status_code == 200, listed_v2.text
        items_v2 = listed_v2.json()
        match_v2 = next(
            (item for item in items_v2 if item["sandboxID"] == sandbox_id),
            None,
        )
        assert match_v2 is not None
        assert LISTED_SANDBOX_FIELDS <= set(match_v2)
    finally:
        httpx.delete(
            f"{api_url()}/sandboxes/{sandbox_id}",
            headers=api_headers(),
            timeout=30.0,
        )
