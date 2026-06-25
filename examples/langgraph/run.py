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

examples_root = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(examples_root))
sys.path.insert(0, str(Path(__file__).resolve().parent))

from alert_graph import run_alert_graph_from_file
from support.python_template import SANDBOX_TEMPLATE_ENV, ensure_python_template, sandbox_template_name


def _sample_path() -> Path:
    return Path(__file__).resolve().parent / "data" / "sample_alert.json"


def main() -> None:
    path = sys.argv[1] if len(sys.argv) > 1 else str(_sample_path())
    if not os.environ.get(SANDBOX_TEMPLATE_ENV):
        ensure_python_template()
    template_name = os.environ.get(SANDBOX_TEMPLATE_ENV) or sandbox_template_name()
    result = run_alert_graph_from_file(path, template_name=template_name)
    print(result["summary"])


if __name__ == "__main__":
    main()
