"""Типизированная интроспекция Postgres стека (порт проброшен на 127.0.0.1:5432).

Только чтение для ассертов side-effects. Схема — migrations/; таблицы китов golibinfra: сессии
refresh — authnkit/session/pgstore (authn_sessions, authn_session_tokens), outbox_<domain> —
outboxkit/pgstore (TableMigrations), bruteforce_attempts — bruteforcekit/pgstore.
"""

from __future__ import annotations

import os
from collections.abc import Sequence
from types import TracebackType
from typing import Any

import psycopg
from psycopg.rows import dict_row

DEFAULT_DSN = os.environ.get(
    "E2E_PG_DSN", "postgresql://postgres:postgres@127.0.0.1:5432/app"
)


class Pg:
    def __init__(self, dsn: str = DEFAULT_DSN) -> None:
        self._conn: psycopg.Connection[dict[str, Any]] = psycopg.connect(
            dsn, autocommit=True, row_factory=dict_row
        )

    def __enter__(self) -> Pg:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        self._conn.close()

    def close(self) -> None:
        self._conn.close()

    # --- low-level ---

    def rows(
        self, sql: str, params: Sequence[Any] = ()
    ) -> list[dict[str, Any]]:
        with self._conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.fetchall()

    def row(self, sql: str, params: Sequence[Any] = ()) -> dict[str, Any] | None:
        with self._conn.cursor() as cur:
            cur.execute(sql, params)
            return cur.fetchone()

    def scalar(self, sql: str, params: Sequence[Any] = ()) -> Any:
        r = self.row(sql, params)
        return None if r is None else next(iter(r.values()))

    # --- domain helpers (schema-coupled, осознанно при «максимуме») ---

    def users_row(self, email: str) -> dict[str, Any] | None:
        return self.row(
            "SELECT * FROM users WHERE LOWER(email) = LOWER(%s)", [email]
        )

    def sessions_for(self, user_id: str) -> list[dict[str, Any]]:
        """Сессии refresh пользователя: revoked_at, число токенов цепочки и живых (непогашенных)."""
        return self.rows(
            "SELECT s.id, s.revoked_at, count(t.token_hash) AS tokens, "
            "count(t.token_hash) FILTER (WHERE t.used_at IS NULL) AS live_tokens "
            "FROM authn_sessions s "
            "LEFT JOIN authn_session_tokens t ON t.session_id = s.id "
            "WHERE s.subject = %s GROUP BY s.id ORDER BY s.created_at",
            [str(user_id)],
        )

    def bruteforce_for(self, subject: str) -> list[dict[str, Any]]:
        return self.rows(
            "SELECT * FROM bruteforce_attempts WHERE subject = %s", [subject]
        )

    def email_verifications_for(self, user_id: str) -> list[dict[str, Any]]:
        return self.rows(
            "SELECT * FROM email_verifications WHERE user_id = %s "
            "ORDER BY created_at",
            [user_id],
        )
