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

"""Kind cluster helpers for template-build E2E cleanup and readiness."""

from __future__ import annotations

import subprocess


def actor_name_for_tag(template_id: str, tag: str) -> str:
    """Mirror templateref.ActorNameForTag for local ActorTemplate names."""
    template_id = template_id.strip()
    tag = tag.strip().lower()
    if not tag:
        return template_id
    out: list[str] = []
    for ch in tag:
        if ("a" <= ch <= "z") or ("0" <= ch <= "9") or ch == "-":
            out.append(ch)
        elif ch in {"_", "."}:
            out.append("-")
    sanitized = "".join(out).strip("-")
    if not sanitized:
        return template_id
    if len(sanitized) > 32:
        sanitized = sanitized[:32]
    return f"{template_id}--{sanitized}"


def wait_actortemplate_ready(
    name: str,
    *,
    namespace: str = "actordock",
    timeout_sec: int = 120,
) -> None:
    proc = subprocess.run(
        [
            "kubectl",
            "wait",
            "--for=condition=Ready",
            f"actortemplate/{name}",
            "-n",
            namespace,
            f"--timeout={timeout_sec}s",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
    if proc.returncode != 0:
        detail = (proc.stderr or proc.stdout or "").strip()
        raise RuntimeError(f"actortemplate/{name} not ready: {detail}")


def delete_actortemplates(*names: str, namespace: str = "actordock") -> None:
    existing = [name for name in names if name]
    if not existing:
        return
    subprocess.run(
        [
            "kubectl",
            "delete",
            "actortemplate",
            *existing,
            "-n",
            namespace,
            "--ignore-not-found",
        ],
        check=False,
        capture_output=True,
        text=True,
    )
