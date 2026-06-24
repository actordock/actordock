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

"""Run PTO calculation inside an Actordock sandbox (E2B SDK)."""

from __future__ import annotations

import time

from typing import Callable, TypeVar

from e2b import Sandbox
from e2b.exceptions import TimeoutException

DAYS_PER_YEAR = 6
SANDBOX_TEMPLATE = "base"
CALC_SCRIPT = """\
#!/bin/sh
tenure="$1"
echo $((tenure * 6))
"""

DEFAULT_MAX_ATTEMPTS = 15
DEFAULT_RETRY_DELAY_SEC = 0.5

T = TypeVar("T")


def _retry_envd(fn: Callable[[], T]) -> T:
    """Retry envd RPC until sandbox is reachable (may still be starting)."""
    last_err: Exception | None = None
    for attempt in range(DEFAULT_MAX_ATTEMPTS):
        try:
            return fn()
        except (TimeoutException, OSError) as err:
            last_err = err
            if attempt + 1 >= DEFAULT_MAX_ATTEMPTS:
                raise
            time.sleep(DEFAULT_RETRY_DELAY_SEC)
    assert last_err is not None
    raise last_err


def _write_with_retry(sandbox: Sandbox, path: str, content: str) -> None:
    _retry_envd(lambda: sandbox.files.write(path, content))


def _run_with_retry(sandbox: Sandbox, cmd: str) -> str:
    return _retry_envd(lambda: sandbox.commands.run(cmd).stdout.strip())


def calculate_pto(tenure_years: int) -> int:
    """Compute PTO days for tenure using policy rate (6 days/year) in a sandbox."""
    if tenure_years < 0:
        raise ValueError("tenure_years must be non-negative")

    sandbox = Sandbox.create(template=SANDBOX_TEMPLATE, secure=False, timeout=120)
    try:
        _write_with_retry(sandbox, "/tmp/pto_calc.sh", CALC_SCRIPT)
        stdout = _run_with_retry(sandbox, f"sh /tmp/pto_calc.sh {tenure_years}")
        return int(stdout)
    finally:
        sandbox.kill()
