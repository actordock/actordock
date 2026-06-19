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

"""E2B SDK environment for local Actordock (see hack/.env.local)."""

from __future__ import annotations

import os
from typing import Iterable

REQUIRED_ENV_VARS: tuple[str, ...] = (
    "E2B_API_URL",
    "E2B_SANDBOX_URL",
    "E2B_DOMAIN",
    "E2B_API_KEY",
    "E2B_VALIDATE_API_KEY",
)


def missing_env_vars(names: Iterable[str] = REQUIRED_ENV_VARS) -> list[str]:
    return [name for name in names if not os.environ.get(name)]


def ensure_e2b_env() -> None:
    missing = missing_env_vars()
    if missing:
        raise RuntimeError(
            "missing required environment variables: "
            + ", ".join(missing)
            + " (run: source hack/.env.local after ./hack/install-local.sh)"
        )
