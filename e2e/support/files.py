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

"""Retry helpers for envd filesystem RPC (sandbox may still be starting)."""

from __future__ import annotations

import time
from typing import TYPE_CHECKING, Any

from e2b.exceptions import TimeoutException

if TYPE_CHECKING:
    from e2b import Sandbox

DEFAULT_MAX_ATTEMPTS = 15
DEFAULT_RETRY_DELAY_SEC = 0.5


def write_file(
    sandbox: Sandbox,
    path: str,
    content: str,
    *,
    max_attempts: int = DEFAULT_MAX_ATTEMPTS,
    retry_delay_sec: float = DEFAULT_RETRY_DELAY_SEC,
    **kwargs: Any,
) -> Any:
    last_err: Exception | None = None
    for attempt in range(max_attempts):
        try:
            return sandbox.files.write(path, content, **kwargs)
        except (TimeoutException, OSError) as err:
            last_err = err
            if attempt + 1 >= max_attempts:
                raise
            time.sleep(retry_delay_sec)
    assert last_err is not None
    raise last_err


def read_file(
    sandbox: Sandbox,
    path: str,
    *,
    max_attempts: int = DEFAULT_MAX_ATTEMPTS,
    retry_delay_sec: float = DEFAULT_RETRY_DELAY_SEC,
    **kwargs: Any,
) -> str:
    last_err: Exception | None = None
    for attempt in range(max_attempts):
        try:
            return sandbox.files.read(path, **kwargs)
        except (TimeoutException, OSError) as err:
            last_err = err
            if attempt + 1 >= max_attempts:
                raise
            time.sleep(retry_delay_sec)
    assert last_err is not None
    raise last_err


def list_dir(
    sandbox: Sandbox,
    path: str,
    *,
    max_attempts: int = DEFAULT_MAX_ATTEMPTS,
    retry_delay_sec: float = DEFAULT_RETRY_DELAY_SEC,
    **kwargs: Any,
) -> list[Any]:
    last_err: Exception | None = None
    for attempt in range(max_attempts):
        try:
            return sandbox.files.list(path, **kwargs)
        except (TimeoutException, OSError) as err:
            last_err = err
            if attempt + 1 >= max_attempts:
                raise
            time.sleep(retry_delay_sec)
    assert last_err is not None
    raise last_err
