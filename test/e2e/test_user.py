"""E2E-пример: login → /me → выборка по id + сверка с БД. Шаблон для форков."""

import pytest

from client.http import Api
from conftest import RegisteredUser
from infra.pg import Pg
from scenarios.auth import login
from scenarios.user import get_user


@pytest.mark.e2e
def test_user_lookup_by_id(api: Api, user: RegisteredUser, pg: Pg) -> None:
    login(api, user.email, user.password)
    me = api.me()
    fetched = get_user(api, me.id)
    assert fetched.id == me.id
    assert fetched.email == user.email

    # HTTP-контракт сверяем с реальной строкой в users
    row = pg.users_row(user.email)
    assert row is not None
    assert str(row["id"]) == me.id
    assert row["status"] == "active"
    assert row["version"] == 1
    assert row["email_verified"] is False
