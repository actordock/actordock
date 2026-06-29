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

# Debian-based Antigravity runtime with the statically linked envd binary copied
# from the official Actordock base image. google-antigravity ships manylinux
# wheels and does not install on the Alpine-based envd rootfs.

ARG ENVD_IMAGE
FROM ${ENVD_IMAGE} AS envd-src

FROM python:3.13-slim
WORKDIR /app

COPY --from=envd-src /ko-app/envd /ko-app/envd

COPY sandbox_requirements.txt agent.py /app/
RUN pip install --no-cache-dir -r /app/sandbox_requirements.txt

ENV PYTHONUNBUFFERED=1
