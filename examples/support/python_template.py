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

"""Shared helpers for Actordock example sandboxes."""

from __future__ import annotations

import os
import uuid

from e2b import Template

SANDBOX_TEMPLATE_ENV = "ACTORDOCK_SANDBOX_TEMPLATE"

PYTHON_APK_CMD = "apk add --no-cache python3 py3-pip"


def sandbox_template_name() -> str:
    """Return the sandbox template name for examples (from env or built default)."""
    return os.environ[SANDBOX_TEMPLATE_ENV]


def build_python_template(
    name: str | None = None,
    *,
    cpu_count: int = 2,
    memory_mb: int = 512,
) -> str:
    """Build a Python-enabled template from the official base template."""
    template_name = name or f"example-py-{uuid.uuid4().hex[:8]}"
    spec = Template().from_template("base").run_cmd(PYTHON_APK_CMD)
    Template.build(spec, template_name, cpu_count=cpu_count, memory_mb=memory_mb)
    return template_name


def ensure_python_template(
    name: str | None = None,
    *,
    cpu_count: int = 2,
    memory_mb: int = 512,
) -> str:
    """Build a Python template and export its name via ACTORDOCK_SANDBOX_TEMPLATE."""
    template_name = build_python_template(name, cpu_count=cpu_count, memory_mb=memory_mb)
    os.environ[SANDBOX_TEMPLATE_ENV] = template_name
    return template_name
