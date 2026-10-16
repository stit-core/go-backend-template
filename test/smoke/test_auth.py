"""Smoke: cookie-based auth-флоу + side-effect ассерты в Postgres.

register/login/me/refresh-rotation/reuse-detect/logout, сессии refresh
(authn_sessions кита authnkit/session), bruteforce_attempts счётчик.
"""

from typing import Any

import pytest

from client.http import Api
from client.models import ChangePasswordRequest
from conftest import RegisteredUser
from infra.pg import Pg
from scenarios.auth import login, replay_refresh_status, rotate_refresh


def _open(sessions: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [s for s in sessions if s["revoked_at"] is None]


@pytest.mark.smoke
def test_auth_cookie_flow(
    api: Api, user: RegisteredUser, stack: str, pg: Pg
) -> None:
    urow = pg.users_row(user.email)
    assert urow is not None
    uid = urow["id"]

    # 1. login → 200 + cookies; в БД одна открытая сессия с одним живым токеном
    login(api, user.email, user.password)
    sessions = pg.sessions_for(uid)
    assert len(_open(sessions)) == len(sessions) == 1
    assert sessions[0]["live_tokens"] == 1

    # 2. /me под cookie — email совпадает
    assert api.me().email == user.email

    # 3. refresh → 204, ротация в той же сессии: прежний токен погашен, следующий жив
    old, new = rotate_refresh(api)
    assert old is not None and new is not None
    assert new != old, "refresh_token did not rotate"
    sessions = pg.sessions_for(uid)
    assert len(sessions) == 1
    assert (sessions[0]["tokens"], sessions[0]["live_tokens"]) == (2, 1)

    # 4. повтор погашенного refresh-cookie из отдельной сессии, когда следующий токен уже обновил
    #    сессию → 401: сессия закрыта целиком, последний токен цепочки тоже мёртв (повтор до
    #    того — в окне SESSION_RETRY_WINDOW — выдал бы новый токен: так повторяет клиент, чей
    #    ответ потерян)
    rotate_refresh(api)
    with Api(stack) as replay:
        assert replay_refresh_status(replay, old) == 401
    assert _open(pg.sessions_for(uid)) == []
    assert api.status("POST", "/api/auth/refresh") == 401

    # 5. /me ещё 200: access-токен живёт свой срок
    assert api.me().email == user.email

    # 6. новый вход → logout → 204; /me → 401; открытых сессий в БД не осталось
    login(api, user.email, user.password)
    assert len(_open(pg.sessions_for(uid))) == 1
    api.logout()
    assert api.status("GET", "/api/users/me") == 401
    assert _open(pg.sessions_for(uid)) == []


@pytest.mark.smoke
def test_bruteforce_counter_persisted(
    api: Api, user: RegisteredUser, pg: Pg
) -> None:
    assert pg.bruteforce_for(user.email) == []
    assert (
        api.status(
            "POST",
            "/api/auth/login",
            json={"email": user.email, "password": "wrong-password"},
        )
        == 401
    )
    rows = pg.bruteforce_for(user.email)
    assert len(rows) >= 1
    assert sum(int(r["failed_count"]) for r in rows) >= 1


@pytest.mark.smoke
def test_bruteforce_lock(
    api: Api, user: RegisteredUser, other_user: RegisteredUser, pg: Pg
) -> None:
    # BRUTEFORCE_MAX_FAILURES=5: давим неверным паролем пока не словим 429
    got_lock = False
    for _ in range(8):
        if (
            api.status(
                "POST",
                "/api/auth/login",
                json={"email": user.email, "password": "nope"},
            )
            == 429
        ):
            got_lock = True
            break
    assert got_lock, "lock (429) не наступил за 8 попыток при MAX_FAILURES=5"

    rows = pg.bruteforce_for(user.email)
    assert any(r["locked_until"] is not None for r in rows)

    # верный пароль во время лока тоже 429 (lock перекрывает)
    assert (
        api.status(
            "POST",
            "/api/auth/login",
            json={"email": user.email, "password": user.password},
        )
        == 429
    )
    # другой свежий email не залочен — один неверный даёт 401, не 429
    assert (
        api.status(
            "POST",
            "/api/auth/login",
            json={"email": other_user.email, "password": "nope"},
        )
        == 401
    )


@pytest.mark.smoke
def test_change_password(
    api: Api, user: RegisteredUser, stack: str
) -> None:
    login(api, user.email, user.password)
    new_password = "new-pass-123456"

    # неверный old → 422 поля old_password: сессия цела, входить заново не нужно
    assert (
        api.status(
            "POST",
            "/api/auth/change-password",
            json={
                "old_password": "WRONG-old",
                "new_password": new_password,
            },
        )
        == 422
    )
    # верный old → 204; смена пароля закрывает все сессии учётной записи — refresh этой → 401
    api.change_password(
        ChangePasswordRequest(old_password=user.password, new_password=new_password)
    )
    assert api.status("POST", "/api/auth/refresh") == 401
    # старый пароль больше не логинит, новый — да
    with Api(stack) as fresh:
        assert (
            fresh.status(
                "POST",
                "/api/auth/login",
                json={"email": user.email, "password": user.password},
            )
            == 401
        )
    with Api(stack) as fresh2:
        fresh2.expect(
            "POST",
            "/api/auth/login",
            expected=200,
            json={"email": user.email, "password": new_password},
        )
