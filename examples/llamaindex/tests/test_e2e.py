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

import pytest

from workflow import run_policy_workflow


@pytest.mark.timeout(180)
def test_workflow_retrieves_and_calls_actordock(demo_template_name: str) -> None:
    answer = run_policy_workflow("PTO accrual for tenure?", 3, demo_template_name)
    assert "18" in answer
    assert "PTO days" in answer
