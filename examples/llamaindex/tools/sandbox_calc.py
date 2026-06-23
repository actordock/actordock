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

from e2b import Sandbox
from e2b.exceptions import TimeoutException

DAYS_PER_YEAR = 6
CALC_SCRIPT = """\
import sys
tenure = int(sys.argv[1])
days_per_year = 6
print(tenure * days_per_year)
"""

DEFAULT_MAX_ATTEMPTS = 15
DEFAULT_RETRY_DELAY_SEC = 0.5


def _run_with_retry(sandbox: Sandbox, cmd: str) -> str:
    last_err: Exception | None = None
    for attempt in range(DEFAULT_MAX_ATTEMPTS):
        try:
            result = sandbox.commands.run(cmd)
            return result.stdout.strip()
        except TimeoutException as err:
            last_err = err
            if attempt + 1 >= DEFAULT_MAX_ATTEMPTS:
                raise
            time.sleep(DEFAULT_RETRY_DELAY_SEC)
    assert last_err is not None
    raise last_err


def calculate_pto(tenure_years: int) -> int:
    """Compute PTO days for tenure using policy rate (6 days/year) in a sandbox."""
    if tenure_years < 0:
        raise ValueError("tenure_years must be non-negative")

    sandbox = Sandbox.create(template="base", secure=False, timeout=120)
    try:
        sandbox.files.write("/tmp/pto_calc.py", CALC_SCRIPT)
        stdout = _run_with_retry(sandbox, f"python3 /tmp/pto_calc.py {tenure_years}")
        return int(stdout)
    finally:
        sandbox.kill()
