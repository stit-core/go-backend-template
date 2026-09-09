"""Typed-ассерты по per-domain outbox-таблицам (golibinfra/outboxkit/pgstore,
`TableMigrations`): subject `<domain>.events.<event>` живёт в `outbox_<domain>`.

Колонки: id, subject, payload (JSONB → dict psycopg'ом), created_at,
published_at (NULL пока строку не сдренил worker в NATS).
"""

from __future__ import annotations

import re
import time
from collections.abc import Callable
from typing import Any

from infra.pg import Pg

Predicate = Callable[[dict[str, Any]], bool]

_DOMAIN = re.compile(r"^[a-z][a-z0-9_]*$")


def table_for(subject: str) -> str:
    """`user.events.registered` → `outbox_user` (per-domain таблица источника)."""
    domain = subject.split(".", 1)[0]
    if not _DOMAIN.match(domain):
        raise ValueError(f"outbox: subject без domain-префикса: {subject!r}")
    return f"outbox_{domain}"


def find(
    pg: Pg, subject: str, *, predicate: Predicate | None = None
) -> list[dict[str, Any]]:
    rows = pg.rows(
        "SELECT id, subject, payload, created_at, published_at "
        f"FROM {table_for(subject)} WHERE subject = %s ORDER BY created_at",
        [subject],
    )
    if predicate is None:
        return rows
    return [r for r in rows if predicate(r["payload"])]


def wait_for(
    pg: Pg,
    subject: str,
    *,
    predicate: Predicate | None = None,
    timeout: float = 5.0,
    published: bool = False,
    interval: float = 0.2,
) -> dict[str, Any]:
    """Поллит outbox пока не появится строка по subject (+predicate; +published_at
    IS NOT NULL если published=True). Возвращает последнюю; иначе AssertionError.
    """
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        hits = find(pg, subject, predicate=predicate)
        if published:
            hits = [r for r in hits if r["published_at"] is not None]
        if hits:
            return hits[-1]
        time.sleep(interval)
    raise AssertionError(
        f"outbox: нет строки subject={subject} "
        f"(predicate={'set' if predicate else 'none'}, published={published}) "
        f"за {timeout}s"
    )
