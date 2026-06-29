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

"""Build an Antigravity-enabled sandbox template for Actordock."""

from __future__ import annotations

import os
import subprocess
import uuid
from pathlib import Path

from e2b import Sandbox, Template

from support.python_template import SANDBOX_TEMPLATE_ENV, SANDBOX_TIMEOUT_SEC

AGENT_PATH = "/app/agent.py"
EXAMPLE_DIR = Path(__file__).resolve().parent
BASE_IMAGE_ENV = "ACTORDOCK_ANTIGRAVITY_BASE_IMAGE"
ENVD_IMAGE_ENV = "ACTORDOCK_ENVD_IMAGE"
DEFAULT_BASE_IMAGE = "localhost:5001/actordock/antigravity-base:latest"


def _envd_image() -> str:
    if image := os.environ.get(ENVD_IMAGE_ENV):
        return image
    try:
        out = subprocess.run(
            [
                "kubectl",
                "get",
                "actortemplate",
                "base",
                "-n",
                "actordock",
                "-o",
                "jsonpath={.spec.containers[0].image}",
            ],
            check=True,
            capture_output=True,
            text=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as err:
        raise RuntimeError(
            f"set {ENVD_IMAGE_ENV} or ensure kubectl can read the base ActorTemplate"
        ) from err
    image = out.stdout.strip()
    if not image:
        raise RuntimeError("base ActorTemplate envd image is empty")
    return image


def _image_exists(tag: str) -> bool:
    local = subprocess.run(
        ["docker", "image", "inspect", tag],
        capture_output=True,
        check=False,
    )
    if local.returncode == 0:
        return True
    remote = subprocess.run(
        ["docker", "manifest", "inspect", tag],
        capture_output=True,
        check=False,
    )
    return remote.returncode == 0


def ensure_antigravity_base_image() -> str:
    """Build and push the Debian+envd+Antigravity image to the local Kind registry."""
    if image := os.environ.get(BASE_IMAGE_ENV):
        return image

    tag = DEFAULT_BASE_IMAGE
    if not _image_exists(tag):
        envd_image = _envd_image()
        subprocess.run(
            [
                "docker",
                "build",
                "--build-arg",
                f"ENVD_IMAGE={envd_image}",
                "-t",
                tag,
                str(EXAMPLE_DIR),
            ],
            check=True,
        )
    subprocess.run(["docker", "push", tag], check=True)
    os.environ[BASE_IMAGE_ENV] = tag
    return tag


def build_antigravity_template(
    name: str | None = None,
    *,
    cpu_count: int = 2,
    memory_mb: int = 512,
) -> str:
    """Register a template backed by the Antigravity base image."""
    template_name = name or f"example-antigravity-{uuid.uuid4().hex[:8]}"
    base_image = ensure_antigravity_base_image()
    spec = Template().from_image(base_image)
    Template.build(spec, template_name, cpu_count=cpu_count, memory_mb=memory_mb)
    return template_name


def ensure_antigravity_template(
    name: str | None = None,
    *,
    cpu_count: int = 2,
    memory_mb: int = 512,
) -> str:
    """Build the Antigravity template and export its name via ACTORDOCK_SANDBOX_TEMPLATE."""
    template_name = build_antigravity_template(name, cpu_count=cpu_count, memory_mb=memory_mb)
    os.environ[SANDBOX_TEMPLATE_ENV] = template_name
    return template_name


def create_antigravity_sandbox(
    template_name: str,
    *,
    envs: dict[str, str] | None = None,
) -> Sandbox:
    """Create a sandbox with shared example defaults."""
    return Sandbox.create(
        template=template_name,
        secure=False,
        timeout=SANDBOX_TIMEOUT_SEC,
        envs=envs,
    )
