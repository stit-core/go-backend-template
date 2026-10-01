"""Write-ручки user-домена: PATCH (поля целиком, optimistic version) и DELETE (soft).
Side-effects сверяем в Postgres + outbox.
"""

import uuid

import pytest

from client.http import Api
from client.models import UserUpdateRequest
from conftest import RegisteredUser
from infra import outbox
from infra.pg import Pg
from scenarios.auth import login


@pytest.mark.e2e
def test_patch_user(api: Api, user: RegisteredUser, admin: Api, pg: Pg) -> None:
    login(api, user.email, user.password)
    me = api.me()
    path = f"/api/users/{me.id}"

    # клиент меняет своё имя; version для следующего PATCH — из ответа
    renamed = api.update_user(me.id, UserUpdateRequest(name="Renamed", version=me.version))
    assert (renamed.name, renamed.version) == ("Renamed", me.version + 1)
    # свой статус клиент не меняет → 403, имя не применяется
    own_status = {"name": "Lost", "status": "active", "version": renamed.version}
    assert api.status("PATCH", path, json=own_status) == 403

    # admin: name и status — одной записью, version++, outbox user.events.updated
    blocked = admin.update_user(
        me.id, UserUpdateRequest(name="Blocked", status="blocked", version=renamed.version)
    )
    assert (blocked.name, blocked.status, blocked.version) == (
        "Blocked",
        "blocked",
        renamed.version + 1,
    )
    row = pg.users_row(user.email)
    assert row is not None
    assert (row["name"], row["status"], row["version"]) == ("Blocked", "blocked", blocked.version)
    ev = outbox.wait_for(pg, "user.events.updated", predicate=lambda p: p.get("ID") == me.id)
    assert ev["payload"]["ID"] == me.id

    # устаревшая version → 409; неизвестный статус → 422; пустой PATCH → 400; чужой id → 404
    assert admin.status("PATCH", path, json={"name": "Again", "version": renamed.version}) == 409
    unknown_status = {"status": "deleted", "version": blocked.version}
    assert admin.status("PATCH", path, json=unknown_status) == 422
    assert admin.status("PATCH", path, json={"version": blocked.version}) == 400
    assert (
        admin.status("PATCH", f"/api/users/{uuid.uuid4()}", json={"name": "Ghost", "version": 1})
        == 404
    )


@pytest.mark.e2e
def test_delete_user(api: Api, user: RegisteredUser, admin: Api, pg: Pg) -> None:
    row = pg.users_row(user.email)
    assert row is not None
    uid = str(row["id"])

    admin.expect("DELETE", f"/api/users/{uid}", expected=204)
    gone = pg.users_row(user.email)  # users_email_uq partial: status!='deleted'
    assert gone is None or gone["status"] == "deleted"
    ev = outbox.wait_for(pg, "user.events.deleted", predicate=lambda p: p.get("ID") == uid)
    assert ev["payload"]["ID"] == uid

    # повторный GET → 404; удаление и правка удалённого → 404
    assert admin.status("GET", f"/api/users/{uid}") == 404
    assert admin.status("DELETE", f"/api/users/{uid}") == 404
    assert admin.status("PATCH", f"/api/users/{uid}", json={"name": "Revived", "version": 2}) == 404
