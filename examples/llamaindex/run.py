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

"""CLI demo: policy RAG + Actordock sandbox PTO calculator."""

from __future__ import annotations

import argparse

from workflow import answer_pto_question


def main() -> None:
    parser = argparse.ArgumentParser(description="PTO policy Q&A with LlamaIndex + Actordock")
    parser.add_argument(
        "--question",
        default="How does PTO accrual work for tenure?",
        help="Natural language question for retrieval",
    )
    parser.add_argument("--tenure-years", type=int, default=3, help="Years of tenure to calculate")
    args = parser.parse_args()
    print(answer_pto_question(args.question, args.tenure_years))


if __name__ == "__main__":
    main()
