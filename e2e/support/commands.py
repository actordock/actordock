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

"""Retry helpers for envd commands (create/resume may return before envd is ready)."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING, Any

from e2b.exceptions import TimeoutException

if TYPE_CHECKING:
    from e2b import Sandbox
    from e2b.sandbox_sync.commands.command import CommandResult

DEFAULT_MAX_ATTEMPTS = 15
DEFAULT_RETRY_DELAY_SEC = 0.5


def run_command(
    sandbox: Sandbox,
    cmd: str,
    *,
    max_attempts: int = DEFAULT_MAX_ATTEMPTS,
    retry_delay_sec: float = DEFAULT_RETRY_DELAY_SEC,
    **kwargs: Any,
) -> CommandResult:
    """Run a sandbox command, retrying while Router/envd is still starting."""
    last_err: Exception | None = None
    for attempt in range(max_attempts):
        try:
            return sandbox.commands.run(cmd, **kwargs)
        except TimeoutException as err:
            last_err = err
            if attempt + 1 >= max_attempts:
                raise
            time.sleep(retry_delay_sec)
    assert last_err is not None
    raise last_err
