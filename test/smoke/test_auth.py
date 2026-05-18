"""Smoke: cookie-based auth-флоу + side-effect ассерты в Postgres.

register/login/me/refresh-rotation/reuse-detect/logout/expired,
refresh_sessions state-машина, bruteforce_attempts счётчик.
"""

import pytest

from client.http import Api
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
