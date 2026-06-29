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

"""Run the Antigravity agent inside an Actordock sandbox via the E2B SDK."""

from __future__ import annotations

import os
import shlex

from e2b import Sandbox

from support.python_template import SANDBOX_TEMPLATE_ENV, sandbox_template_name
from template_build import AGENT_PATH, create_antigravity_sandbox


def _gemini_env() -> dict[str, str]:
    api_key = os.environ.get("GEMINI_API_KEY") or os.environ.get("GOOGLE_API_KEY")
    if not api_key:
        raise RuntimeError(
            "GEMINI_API_KEY (or GOOGLE_API_KEY) is required to run the Antigravity agent"
        )
    return {"GEMINI_API_KEY": api_key}


def _default_template_name() -> str:
    try:
        return sandbox_template_name()
    except KeyError as err:
        raise RuntimeError(
            f"missing {SANDBOX_TEMPLATE_ENV}; run ./hack/verify-examples.sh or set the env var"
        ) from err


def run_antigravity_agent(
    prompt: str,
    *,
    template_name: str | None = None,
    sandboxes: list[Sandbox] | None = None,
) -> str:
    """Create a sandbox, run agent.py with the prompt, and return stdout."""
    if template_name is None:
        template_name = _default_template_name()

    sandbox = create_antigravity_sandbox(template_name, envs=_gemini_env())
    if sandboxes is not None:
        sandboxes.append(sandbox)

    try:
        result = sandbox.commands.run(
            f"python3 {AGENT_PATH} {shlex.quote(prompt)}",
        )
        if result.exit_code != 0:
            raise RuntimeError(
                f"agent exited with code {result.exit_code}: {result.stderr.strip()}"
            )
        return result.stdout
    finally:
        if sandboxes is None:
            sandbox.kill()


def verify_antigravity_import(template_name: str) -> str:
    """Confirm google-antigravity is importable in a sandbox spawned from the template."""
    sandbox = create_antigravity_sandbox(template_name)
    try:
        result = sandbox.commands.run(
            'python3 -c "import google.antigravity; print(google.antigravity.__name__)"',
        )
        if result.exit_code != 0:
            raise RuntimeError(
                f"import check failed with code {result.exit_code}: {result.stderr.strip()}"
            )
        return result.stdout.strip()
    finally:
        sandbox.kill()
