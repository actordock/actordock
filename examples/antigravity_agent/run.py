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

from runner import run_antigravity_agent
from support.python_template import SANDBOX_TEMPLATE_ENV
from template_build import ensure_antigravity_template


def main() -> None:
    prompt = (
        sys.argv[1]
        if len(sys.argv) > 1
        else "What is the weather in New York City?"
    )
    if not os.environ.get(SANDBOX_TEMPLATE_ENV):
        ensure_antigravity_template()
    print(run_antigravity_agent(prompt))


if __name__ == "__main__":
    main()
