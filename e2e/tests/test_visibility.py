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

"""E2B sandbox visibility (get_info / list)."""

from __future__ import annotations

import pytest
from e2b import Sandbox, SandboxState
from e2b.exceptions import SandboxNotFoundException


def test_get_info_after_create(sandbox: Sandbox) -> None:
    info = Sandbox.get_info(sandbox.sandbox_id)
    assert info.sandbox_id == sandbox.sandbox_id
    assert info.state == SandboxState.RUNNING


def test_get_info_not_found_after_kill() -> None:
    sbx = Sandbox.create(template="base", secure=False)
    sandbox_id = sbx.sandbox_id
    sbx.kill()
    with pytest.raises(SandboxNotFoundException):
        Sandbox.get_info(sandbox_id)


def test_list_includes_sandbox(sandbox: Sandbox) -> None:
    paginator = Sandbox.list()
    items = paginator.next_items()
    assert sandbox.sandbox_id in {item.sandbox_id for item in items}


def test_list_multiple_concurrent() -> None:
    first = Sandbox.create(template="base", secure=False)
    second = Sandbox.create(template="base", secure=False)
    try:
        items = Sandbox.list().next_items()
        ids = {item.sandbox_id for item in items}
        assert first.sandbox_id in ids
        assert second.sandbox_id in ids
    finally:
        first.kill()
        second.kill()
