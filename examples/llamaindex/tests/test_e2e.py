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

"""E2E: LlamaIndex retrieval + Actordock sandbox PTO calculator (#88)."""

from __future__ import annotations

import os

import httpx

from tools.sandbox_calc import calculate_pto
from workflow import answer_pto_question, retrieve_policy_snippets


def _api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def _api_headers() -> dict[str, str]:
    return {"X-API-KEY": os.environ["E2B_API_KEY"]}


def test_calculate_pto_three_years() -> None:
    assert calculate_pto(3) == 18


def test_retrieve_contains_accrual() -> None:
    snippets = retrieve_policy_snippets("PTO accrual rate for employee tenure")
    assert snippets
    assert any("accrual" in snippet.lower() for snippet in snippets)


def test_workflow_answer_contains_golden_days() -> None:
    answer = answer_pto_question("How many PTO days for tenure?", tenure_years=3)
    assert "18" in answer
    assert "days" in answer.lower()


def test_sandbox_cleaned_up_after_calculate() -> None:
    before = httpx.get(f"{_api_url()}/sandboxes", headers=_api_headers(), timeout=30.0)
    before.raise_for_status()
    before_ids = {item["sandboxID"] for item in before.json()}

    assert calculate_pto(1) == 6

    after = httpx.get(f"{_api_url()}/sandboxes", headers=_api_headers(), timeout=30.0)
    after.raise_for_status()
    after_ids = {item["sandboxID"] for item in after.json()}
    assert after_ids == before_ids
