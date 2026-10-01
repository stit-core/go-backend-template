"""Smoke: cookie-based auth-флоу + side-effect ассерты в Postgres.

register/login/me/refresh-rotation/reuse-detect/logout/expired,
refresh_sessions state-машина, bruteforce_attempts счётчик.
"""

import pytest

from client.http import Api
from client.models import ChangePasswordRequest
from conftest import RegisteredUser
from infra.pg import Pg
from scenarios.auth import login, replay_refresh_status, rotate_refresh


def _active(sessions: list[dict[str, object]]) -> list[dict[str, object]]:
    return [
        s
        for s in sessions
        if s["used_at"] is None and s["replaced_by_jti"] is None
    ]


@pytest.mark.smoke
def test_auth_cookie_flow(
    api: Api, user: RegisteredUser, stack: str, pg: Pg
) -> None:
    urow = pg.users_row(user.email)
    assert urow is not None
    uid = urow["id"]

    # 1. login → 200 + cookies; в БД ровно одна активная refresh-сессия
    login(api, user.email, user.password)
    sessions = pg.refresh_sessions_for(uid)
    assert len(sessions) == 1
    assert len(_active(sessions)) == 1

    # 2. /me под cookie — email совпадает
    assert api.me().email == user.email

    # 3. refresh → 204, ротация; в БД старая закрыта, новая активна
    old, new = rotate_refresh(api)
    assert old is not None and new is not None
    assert new != old, "refresh_token did not rotate"
    sessions = pg.refresh_sessions_for(uid)
    assert len(sessions) == 2
    assert len(_active(sessions)) == 1, "ровно одна активная сессия"

    # 4. replay старого refresh-cookie из отдельной сессии → 401
    with Api(stack) as replay:
        assert replay_refresh_status(replay, old) == 401

    # 5. /me с ротированной сессией всё ещё 200
    assert api.me().email == user.email

    # 6. logout → 204; /me → 401; активных сессий в БД не осталось
    api.logout()
    assert api.status("GET", "/api/users/me") == 401
    assert _active(pg.refresh_sessions_for(uid)) == []


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
    uid = api.me().id
    new_password = "new-pass-123456"

    # неверный old → 422 поля old_password: сессия цела, входить заново не нужно
    assert (
        api.status(
            "POST",
            "/api/auth/change-password",
            json={
                "user_id": uid,
                "old_password": "WRONG-old",
                "new_password": new_password,
            },
        )
        == 422
    )
    # верный old → 204
    api.change_password(
        ChangePasswordRequest(user_id=uid, old_password=user.password, new_password=new_password)
    )
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
