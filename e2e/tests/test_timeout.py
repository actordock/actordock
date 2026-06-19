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

"""E2B sandbox timeout (create timeout / set_timeout / end_at)."""

from __future__ import annotations

import time
from datetime import datetime, timezone

from e2b import Sandbox


def _seconds_until(end_at: datetime) -> float:
    now = datetime.now(timezone.utc)
    if end_at.tzinfo is None:
        end_at = end_at.replace(tzinfo=timezone.utc)
    return (end_at - now).total_seconds()


def test_create_timeout_sets_end_at() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=60)
    try:
        info = sbx.get_info()
        remaining = _seconds_until(info.end_at)
        assert 55 <= remaining <= 65, f"expected ~60s remaining, got {remaining:.1f}s"
    finally:
        sbx.kill()


def test_set_timeout_extends_end_at() -> None:
    sbx = Sandbox.create(template="base", secure=False, timeout=60)
    try:
        before = sbx.get_info().end_at
        time.sleep(1)
        sbx.set_timeout(120)
        after = sbx.get_info().end_at
        remaining = _seconds_until(after)
        assert after > before
        assert 115 <= remaining <= 125, f"expected ~120s remaining, got {remaining:.1f}s"
    finally:
        sbx.kill()
