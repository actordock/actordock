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
import subprocess
import sys
import uuid
from collections.abc import Iterator
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from workflow import build_python_template  # noqa: E402

REQUIRED_ENV = ("E2B_API_URL", "E2B_SANDBOX_URL", "E2B_DOMAIN", "E2B_API_KEY", "E2B_VALIDATE_API_KEY")


@pytest.fixture(scope="session", autouse=True)
def _require_actordock_env() -> None:
    missing = [name for name in REQUIRED_ENV if not os.environ.get(name)]
    if missing:
        pytest.skip("missing Actordock env; run ./hack/verify-examples.sh")


@pytest.fixture(scope="session")
def demo_template_name() -> Iterator[str]:
    name = f"llamaindex-e2e-{uuid.uuid4().hex[:8]}"
    build_python_template(name)
    yield name
    subprocess.run(
        ["kubectl", "delete", "actortemplate", name, "-n", "actordock", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )
