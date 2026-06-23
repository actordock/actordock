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

"""OpenAPI field sets for v0.1.0 sandbox parity gate."""

from __future__ import annotations

SANDBOX_CREATE_RESPONSE_FIELDS = frozenset(
    {
        "clientID",
        "envdVersion",
        "sandboxID",
        "templateID",
        "domain",
    }
)

SANDBOX_CREATE_SECURE_RESPONSE_FIELDS = SANDBOX_CREATE_RESPONSE_FIELDS | {
    "envdAccessToken",
    "trafficAccessToken",
}

SANDBOX_DETAIL_FIELDS = frozenset(
    {
        "clientID",
        "cpuCount",
        "diskSizeMB",
        "endAt",
        "envdVersion",
        "memoryMB",
        "sandboxID",
        "startedAt",
        "state",
        "templateID",
        "domain",
        "allowInternetAccess",
        "lifecycle",
    }
)

SANDBOX_DETAIL_OPTIONAL_FIELDS = frozenset(
    {
        "alias",
        "network",
        "metadata",
    }
)

# Omitted when unset (Go json omitempty); secure-only fields covered in test_secure.py.
SANDBOX_DETAIL_OMIT_EMPTY_FIELDS = frozenset({"volumeMounts", "envdAccessToken"})

LISTED_SANDBOX_FIELDS = frozenset(
    {
        "clientID",
        "cpuCount",
        "diskSizeMB",
        "endAt",
        "envdVersion",
        "memoryMB",
        "sandboxID",
        "startedAt",
        "state",
        "templateID",
        "lifecycle",
    }
)

LISTED_SANDBOX_OPTIONAL_FIELDS = frozenset(
    {
        "alias",
        "metadata",
    }
)

LISTED_SANDBOX_OMIT_EMPTY_FIELDS = frozenset({"volumeMounts"})
