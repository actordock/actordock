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

"""E2E template build (E2B v3 Template.build) and build/tag REST APIs."""

from __future__ import annotations

import uuid

import httpx
import pytest
from e2b import Sandbox, Template

from support.api import api_headers, api_url
from support.cluster import actor_name_for_tag, delete_actortemplates, wait_actortemplate_ready
from support.commands import run_command


def _assert_httpx_installed(sbx: Sandbox) -> None:
    out = run_command(
        sbx,
        'python3 -c "import httpx; print(httpx.__version__)"',
    )
    assert out.exit_code == 0
    assert out.stdout.strip()


@pytest.mark.timeout(300)
def test_template_build_sdk_and_rest_apis() -> None:
    """Template.build from official python base, REST status/logs/tags, sandbox spawn."""
    template_name = f"e2e-py-{uuid.uuid4().hex[:8]}"
    tag_name = "e2e-prod"
    tagged_actor = actor_name_for_tag(template_name, tag_name)

    try:
        template = (
            Template()
            .from_template("python")
            .run_cmd("apk add --no-cache python3 py3-pip")
            .run_cmd("pip install --no-cache-dir --break-system-packages httpx")
        )

        info = Template.build(template, template_name, cpu_count=2, memory_mb=512)
        assert info.template_id
        assert info.build_id

        api = api_url()
        headers = api_headers()

        status_resp = httpx.get(
            f"{api}/templates/{info.template_id}/builds/{info.build_id}/status",
            headers=headers,
            timeout=30.0,
        )
        assert status_resp.status_code == 200, status_resp.text
        status_body = status_resp.json()
        assert status_body["status"] == "ready"
        assert status_body["templateID"] == info.template_id
        assert status_body["buildID"] == info.build_id
        assert isinstance(status_body.get("logEntries"), list)

        logs_resp = httpx.get(
            f"{api}/templates/{info.template_id}/builds/{info.build_id}/logs",
            headers=headers,
            params={"limit": 50},
            timeout=30.0,
        )
        assert logs_resp.status_code == 200, logs_resp.text
        logs_body = logs_resp.json()
        assert isinstance(logs_body.get("logs"), list)
        assert len(logs_body["logs"]) > 0

        assign_resp = httpx.post(
            f"{api}/templates/tags",
            headers=headers,
            json={"target": template_name, "tags": [tag_name]},
            timeout=30.0,
        )
        assert assign_resp.status_code == 201, assign_resp.text
        assigned = assign_resp.json()
        assert assigned["buildID"] == info.build_id
        assert tag_name in assigned["tags"]

        wait_actortemplate_ready(tagged_actor)

        tags_resp = httpx.get(
            f"{api}/templates/{info.template_id}/tags",
            headers=headers,
            timeout=30.0,
        )
        assert tags_resp.status_code == 200, tags_resp.text
        tags = tags_resp.json()
        assert any(t["tag"] == tag_name and t["buildID"] == info.build_id for t in tags)

        patch_resp = httpx.patch(
            f"{api}/v2/templates/{info.template_id}",
            headers=headers,
            json={"public": True},
            timeout=30.0,
        )
        assert patch_resp.status_code == 200, patch_resp.text
        assert template_name in patch_resp.json().get("names", [])

        sbx = Sandbox.create(template=template_name, secure=False, timeout=300)
        try:
            _assert_httpx_installed(sbx)
        finally:
            sbx.kill()

        tagged_sbx = Sandbox.create(
            template=f"{info.template_id}:{tag_name}",
            secure=False,
            timeout=300,
        )
        try:
            _assert_httpx_installed(tagged_sbx)
        finally:
            tagged_sbx.kill()

        base = Sandbox.create(template="base", secure=False, timeout=120)
        try:
            base_out = run_command(
                base,
                'python3 -c "import httpx" || echo missing',
            )
            assert "missing" in base_out.stdout
        finally:
            base.kill()

        del_resp = httpx.request(
            "DELETE",
            f"{api}/templates/tags",
            headers=headers,
            json={"name": template_name, "tags": [tag_name]},
            timeout=30.0,
        )
        assert del_resp.status_code == 204, del_resp.text

        tags_after = httpx.get(
            f"{api}/templates/{info.template_id}/tags",
            headers=headers,
            timeout=30.0,
        ).json()
        assert not any(t["tag"] == tag_name for t in tags_after)
    finally:
        delete_actortemplates(template_name, tagged_actor)
