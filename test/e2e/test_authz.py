"""Authz-матрица user-домена (internal/domain/user/policy.go):
CanList/CanChangeStatus = admin-only; CanGetByID/CanUpdate/CanDelete = selfOrAdmin.
"""

import pytest

from client.http import Api
from conftest import RegisteredUser
from infra.pg import Pg
from scenarios.auth import login


@pytest.mark.e2e
def test_list_admin_only(api: Api, user: RegisteredUser, admin: Api) -> None:
    login(api, user.email, user.password)
    # client → 403
    assert api.status("GET", "/api/users") == 403
    # admin → 200 + форма списка
    body = admin.expect("GET", "/api/users", expected=200).json()
    assert set(body) >= {"data", "total", "page", "size"}
    assert isinstance(body["data"], list)


@pytest.mark.e2e
def test_get_by_id_self_or_admin(
    api: Api, user: RegisteredUser, other_user: RegisteredUser, admin: Api, pg: Pg
) -> None:
    login(api, user.email, user.password)
    me = api.me()
    other_row = pg.users_row(other_user.email)
    assert other_row is not None
    other_id = str(other_row["id"])

    assert api.status("GET", f"/api/users/{me.id}") == 200      # свой → 200
    assert api.status("GET", f"/api/users/{other_id}") == 403   # чужой → 403
    assert admin.status("GET", f"/api/users/{me.id}") == 200     # admin любой
    assert admin.status("GET", f"/api/users/{other_id}") == 200


@pytest.mark.e2e
def test_update_delete_cross_user_forbidden(
    api: Api, user: RegisteredUser, other_user: RegisteredUser, pg: Pg
) -> None:
    login(api, user.email, user.password)
    other_row = pg.users_row(other_user.email)
    assert other_row is not None
    other_id = str(other_row["id"])

    assert (
        api.status(
            "PATCH",
            f"/api/users/{other_id}",
            json={"name": "hijacked", "version": 1},
        )
        == 403
    )
    assert api.status("DELETE", f"/api/users/{other_id}") == 403

    # целевая строка не изменилась
    after = pg.users_row(other_user.email)
    assert after is not None
    assert after["name"] == "Other User"
    assert after["status"] == "active"
