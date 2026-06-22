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

"""Shared HTTP helpers for Platform REST E2E tests."""

from __future__ import annotations

import os


def api_url() -> str:
    return os.environ["E2B_API_URL"].rstrip("/")


def api_headers(*, api_key: str | None = None) -> dict[str, str]:
    return {
        "X-API-KEY": api_key or os.environ["E2B_API_KEY"],
        "Content-Type": "application/json",
    }
