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

"""E2E filesystem via E2B SDK (v0.1.0 WP1 + WP7)."""

from __future__ import annotations

import uuid

from e2b import Sandbox

from support.commands import run_command
from support.files import list_dir, read_file, write_file


def _tmp_path(name: str) -> str:
    return f"/tmp/actordock-e2e-{name}"


def test_write_read_roundtrip(sandbox: Sandbox) -> None:
    path = _tmp_path(f"read-{uuid.uuid4().hex}.txt")
    content = "hello-filesystem"
    write_file(sandbox, path, content)
    assert read_file(sandbox, path) == content


def test_write_overwrite(sandbox: Sandbox) -> None:
    path = _tmp_path(f"overwrite-{uuid.uuid4().hex}.txt")
    write_file(sandbox, path, "first")
    write_file(sandbox, path, "second")
    assert read_file(sandbox, path) == "second"


def test_list_directory_includes_written_file(sandbox: Sandbox) -> None:
    dirname = _tmp_path(f"dir-{uuid.uuid4().hex}")
    filename = "listed.txt"
    file_path = f"{dirname}/{filename}"
    write_file(sandbox, file_path, "list-me")
    entries = list_dir(sandbox, dirname, depth=1)
    names = {entry.name for entry in entries}
    assert filename in names


def test_command_reads_written_file(sandbox: Sandbox) -> None:
    path = _tmp_path(f"cmd-{uuid.uuid4().hex}.txt")
    content = "data"
    write_file(sandbox, path, content)
    result = run_command(sandbox, f"cat {path}")
    assert result.stdout.strip() == content


def test_nested_list_depth_two(sandbox: Sandbox) -> None:
    root = _tmp_path(f"nested-{uuid.uuid4().hex}")
    write_file(sandbox, f"{root}/a.txt", "a")
    write_file(sandbox, f"{root}/sub/b.txt", "b")
    entries = list_dir(sandbox, root, depth=2)
    paths = {entry.path for entry in entries}
    assert f"{root}/a.txt" in paths
    assert f"{root}/sub/b.txt" in paths
