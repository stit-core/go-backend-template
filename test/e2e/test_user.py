"""E2E-пример: login → /me → выборка по id. Шаблон для новых доменных сценариев."""

import pytest

from client.http import Api
from conftest import RegisteredUser
from scenarios.auth import login
from scenarios.user import get_user


@pytest.mark.e2e
def test_user_lookup_by_id(api: Api, user: RegisteredUser) -> None:
    login(api, user.email, user.password)
    me = api.me()
    fetched = get_user(api, me.id)
    assert fetched.id == me.id
    assert fetched.email == user.email
