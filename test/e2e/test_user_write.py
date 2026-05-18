"""Write-ручки user-домена: PATCH (optimistic version) и DELETE (soft).
Side-effects сверяем в Postgres + outbox.
"""

import pytest

from client.http import Api
from conftest import RegisteredUser
from infra import outbox
from infra.pg import Pg


@pytest.mark.e2e
def test_patch_user(api: Api, user: RegisteredUser, admin: Api, pg: Pg) -> None:
    row = pg.users_row(user.email)
    assert row is not None
    uid = str(row["id"])
    assert row["version"] == 1

    # happy: admin меняет name → 200, version++ , outbox user.events.updated
    admin.expect(
        "PATCH", f"/api/users/{uid}", expected=200,
        json={"name": "Renamed", "version": 1},
    )
    after = pg.users_row(user.email)
    assert after is not None
    assert after["name"] == "Renamed"
    assert after["version"] == 2
    ev = outbox.wait_for(pg, "user.events.updated", predicate=lambda p: p.get("ID") == uid)
    assert ev["payload"]["ID"] == uid

    # stale version → 409
    assert (
        admin.status(
            "PATCH", f"/api/users/{uid}", json={"name": "Again", "version": 1}
        )
        == 409
    )
    # пустой body (ни name, ни status) → 400
    assert (
        admin.status("PATCH", f"/api/users/{uid}", json={"version": 2}) == 400
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

    # повторный GET → 404; удаление несуществующего → 404
    assert admin.status("GET", f"/api/users/{uid}") == 404
    assert admin.status("DELETE", f"/api/users/{uid}") == 404
