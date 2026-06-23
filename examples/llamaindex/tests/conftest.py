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

from __future__ import annotations

import os
import sys
from pathlib import Path

import pytest

EXAMPLE_ROOT = Path(__file__).resolve().parents[1]
if str(EXAMPLE_ROOT) not in sys.path:
    sys.path.insert(0, str(EXAMPLE_ROOT))

REQUIRED_ENV = (
    "E2B_API_URL",
    "E2B_SANDBOX_URL",
    "E2B_DOMAIN",
    "E2B_API_KEY",
    "E2B_VALIDATE_API_KEY",
)


@pytest.fixture(scope="session", autouse=True)
def _require_actordock_env() -> None:
    missing = [name for name in REQUIRED_ENV if not os.environ.get(name)]
    if missing:
        pytest.skip(
            "missing Actordock E2B env (run ./hack/install-local.sh && ./hack/verify-local.sh): "
            + ", ".join(missing)
        )
