"""Core-NATS интроспекция (порт 127.0.0.1:4222).

outboxkit-worker публикует в core NATS (conn.PublishMsg), не JetStream —
реплея нет. Поэтому коллектор подписывается ПЕРВЫМ, затем тест триггерит
HTTP-действие, затем wait_for ждёт доставленное сообщение.

nats-py асинхронный; оборачиваем в фоновый event-loop с sync-API, чтобы
не тащить pytest-asyncio и держать харнес синхронным.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import os
import threading
import time
from collections import deque
from collections.abc import Callable
from types import TracebackType
from typing import Any

import nats
from nats.aio.client import Client
from nats.aio.msg import Msg

Predicate = Callable[[dict[str, Any]], bool]
DEFAULT_URL = os.environ.get("E2E_NATS_URL", "nats://127.0.0.1:4222")


class NatsCollector:
    def __init__(
        self, subjects: list[str], url: str = DEFAULT_URL
    ) -> None:
        self._subjects = subjects
        self._url = url
        self._msgs: deque[tuple[str, dict[str, Any]]] = deque()
        self._lock = threading.Lock()
        self._loop = asyncio.new_event_loop()
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._ready = threading.Event()
        self._nc: Client | None = None

    def _run(self) -> None:
        asyncio.set_event_loop(self._loop)
        self._loop.run_until_complete(self._connect())
        self._ready.set()
        self._loop.run_forever()

    async def _connect(self) -> None:
        self._nc = await nats.connect(self._url)
        for subj in self._subjects:
            await self._nc.subscribe(subj, cb=self._on_msg)

    async def _on_msg(self, msg: Msg) -> None:
        try:
            payload: dict[str, Any] = json.loads(msg.data)
        except ValueError:
            payload = {"_raw": msg.data.decode(errors="replace")}
        with self._lock:
            self._msgs.append((msg.subject, payload))

    def __enter__(self) -> NatsCollector:
        self._thread.start()
        if not self._ready.wait(timeout=10):
            raise AssertionError("nats: collector не подключился за 10s")
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        async def _close() -> None:
            if self._nc is not None:
                await self._nc.drain()

        with contextlib.suppress(Exception):
            asyncio.run_coroutine_threadsafe(_close(), self._loop).result(
                timeout=5
            )
        self._loop.call_soon_threadsafe(self._loop.stop)
        self._thread.join(timeout=5)

    def wait_for(
        self,
        subject: str,
        *,
        predicate: Predicate | None = None,
        timeout: float = 5.0,
        interval: float = 0.1,
    ) -> dict[str, Any]:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            with self._lock:
                for s, p in self._msgs:
                    if s == subject and (predicate is None or predicate(p)):
                        return p
            time.sleep(interval)
        raise AssertionError(
            f"nats: нет сообщения subject={subject} за {timeout}s"
        )
