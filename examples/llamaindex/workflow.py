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

"""LlamaIndex workflow with Actordock FunctionTool."""

from __future__ import annotations

import asyncio
import os
from pathlib import Path

import httpx
from e2b import Sandbox, Template
from llama_index.core import Settings, SimpleDirectoryReader, VectorStoreIndex
from llama_index.core.embeddings import MockEmbedding
from llama_index.core.tools import FunctionTool
from llama_index.core.workflow import StartEvent, StopEvent, Workflow, step

DAYS_PER_YEAR = 6
POLICIES_DIR = Path(__file__).resolve().parent / "data" / "policies"


class PolicyQuery(StartEvent):
    question: str
    tenure_years: int


def build_python_template(name: str) -> str:
    spec = Template().from_template("python").run_cmd("apk add --no-cache python3")
    info = Template.build(spec, name, cpu_count=1, memory_mb=512)
    api = os.environ["E2B_API_URL"].rstrip("/")
    httpx.patch(
        f"{api}/v2/templates/{info.template_id}",
        headers={"X-API-KEY": os.environ["E2B_API_KEY"], "Content-Type": "application/json"},
        json={"public": True},
        timeout=60.0,
    ).raise_for_status()
    return name


def _actordock_tool(template_name: str) -> FunctionTool:
    def calculate_pto(tenure_years: int, template_name: str) -> int:
        sandbox = Sandbox.create(template=template_name, secure=False, timeout=120)
        try:
            out = sandbox.commands.run(
                f'python3 -c "print({tenure_years} * {DAYS_PER_YEAR})"',
            )
            return int(out.stdout.strip())
        finally:
            sandbox.kill()

    return FunctionTool.from_defaults(
        fn=calculate_pto,
        partial_params={"template_name": template_name},
        name="actordock_calculate_pto",
        description="Calculate PTO days in an Actordock sandbox.",
    )


def _retrieve_snippet(question: str) -> str:
    Settings.embed_model = MockEmbedding(embed_dim=8)
    docs = SimpleDirectoryReader(input_dir=str(POLICIES_DIR)).load_data()
    nodes = VectorStoreIndex.from_documents(docs).as_retriever(similarity_top_k=1).retrieve(question)
    if not nodes:
        raise RuntimeError("no policy chunks retrieved")
    return nodes[0].get_content()


class PolicyWorkflow(Workflow):
    def __init__(self, template_name: str, **kwargs: object) -> None:
        super().__init__(**kwargs)
        self._tool = _actordock_tool(template_name)

    @step
    async def answer(self, ev: PolicyQuery) -> StopEvent:
        snippet = _retrieve_snippet(ev.question)
        days = int((await self._tool.acall(tenure_years=ev.tenure_years)).raw_output)
        return StopEvent(result=f"{snippet.splitlines()[0][:80]}… → {days} PTO days")


def run_policy_workflow(question: str, tenure_years: int, template_name: str) -> str:
    async def _run() -> str:
        return await PolicyWorkflow(template_name, timeout=180).run(
            question=question,
            tenure_years=tenure_years,
        )

    return asyncio.run(_run())
