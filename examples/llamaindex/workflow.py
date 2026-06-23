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

"""Policy handbook Q&A: retrieve clauses then compute PTO in Actordock."""

from __future__ import annotations

from pathlib import Path

from llama_index.core import Settings, SimpleDirectoryReader, VectorStoreIndex
from llama_index.core.embeddings import MockEmbedding

from tools.sandbox_calc import calculate_pto

EXAMPLE_ROOT = Path(__file__).resolve().parent
POLICIES_DIR = EXAMPLE_ROOT / "data" / "policies"


def _build_retriever():
    Settings.embed_model = MockEmbedding(embed_dim=8)
    documents = SimpleDirectoryReader(input_dir=str(POLICIES_DIR)).load_data()
    index = VectorStoreIndex.from_documents(documents)
    return index.as_retriever(similarity_top_k=3)


def retrieve_policy_snippets(question: str) -> list[str]:
    """Return top-k policy text chunks for a question."""
    retriever = _build_retriever()
    nodes = retriever.retrieve(question)
    return [node.get_content() for node in nodes]


def answer_pto_question(question: str, tenure_years: int) -> str:
    """Retrieve policy context, run sandbox calculator, return human-readable answer."""
    snippets = retrieve_policy_snippets(question)
    if not any("accrual" in snippet.lower() for snippet in snippets):
        raise RuntimeError("retrieval did not return PTO accrual policy text")

    days = calculate_pto(tenure_years)
    excerpt = next(s for s in snippets if "accrual" in s.lower())
    first_line = excerpt.strip().splitlines()[0][:120]
    return (
        f"Policy excerpt: {first_line}… "
        f"For {tenure_years} years tenure, accrued PTO is {days} days."
    )
