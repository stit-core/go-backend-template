"""GET /api/users (admin): форма ответа, пагинация, status-фильтр, границы."""

import pytest

from client.http import Api
from conftest import RegisteredUser


@pytest.mark.e2e
def test_list_shape_and_pagination(
    user: RegisteredUser, other_user: RegisteredUser, admin: Api
) -> None:
    # user/other_user уже зарегистрированы фикстурами → ≥2 строки
    body = admin.expect("GET", "/api/users?page=1&size=2", expected=200).json()
    assert set(body) >= {"data", "total", "page", "size"}
    assert body["page"] == 1
    assert body["size"] == 2
    assert len(body["data"]) <= 2
    assert body["total"] >= 2

    # status-фильтр: active не пуст, все строки active
    active = admin.expect(
        "GET", "/api/users?status=active&size=100", expected=200
    ).json()
    assert active["data"]
    assert all(u["status"] == "active" for u in active["data"])

    # size > max(100) — huma валидирует границу схемы → 4xx
    assert admin.status("GET", "/api/users?size=101") in (400, 422)
