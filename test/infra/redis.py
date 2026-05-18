"""Redis-интроспекция (порт 127.0.0.1:6379/0).

golibinfra/cachekit подключён в cmd/api (cache + idempotency), но
reference-домены auth/user/verification сейчас Redis не пишут — хелпер
служит connectivity-probe и примитивом для форков, которые задействуют
cachekit (cache-ключи, idempotency-ключи).
"""

from __future__ import annotations

import os
from types import TracebackType
from typing import Any, cast

from redis import Redis

DEFAULT_URL = os.environ.get("E2E_REDIS_URL", "redis://127.0.0.1:6379/0")


class RedisInspect:
    def __init__(self, url: str = DEFAULT_URL) -> None:
        self._c = Redis.from_url(url, decode_responses=True)

    def __enter__(self) -> RedisInspect:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        self._c.close()  # type: ignore[no-untyped-call]

    def ping(self) -> bool:
        return bool(self._c.ping())

    def get(self, key: str) -> str | None:
        raw = self._c.get(key)
        return None if raw is None else str(raw)

    def exists(self, key: str) -> bool:
        return bool(self._c.exists(key))

    def ttl(self, key: str) -> int:
        return cast(int, self._c.ttl(key))

    def keys(self, pattern: str = "*") -> list[str]:
        return [str(k) for k in cast("list[Any]", self._c.keys(pattern))]
