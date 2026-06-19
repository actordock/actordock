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

"""E2B sandbox auto-cleanup (scheduler enforces expires_at)."""

from __future__ import annotations

import time

import pytest
from e2b import Sandbox
from e2b.exceptions import SandboxNotFoundException


def test_scheduler_kills_expired_sandbox() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=30)
    sandbox_id = sbx.sandbox_id
    # 30s TTL + scheduler poll interval (~5s) + margin
    time.sleep(40)
    with pytest.raises(SandboxNotFoundException):
        Sandbox.get_info(sandbox_id)


def test_set_timeout_prevents_early_cleanup() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=30)
    sandbox_id = sbx.sandbox_id
    try:
        time.sleep(10)
        sbx.set_timeout(120)
        time.sleep(30)
        info = Sandbox.get_info(sandbox_id)
        assert info.sandbox_id == sandbox_id
    finally:
        sbx.kill()


def test_manual_kill_before_expiry() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=60)
    sandbox_id = sbx.sandbox_id
    sbx.kill()
    with pytest.raises(SandboxNotFoundException):
        Sandbox.get_info(sandbox_id)
    time.sleep(10)
    with pytest.raises(SandboxNotFoundException):
        Sandbox.get_info(sandbox_id)
