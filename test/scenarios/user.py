"""Step-функции user-домена. Паттерн для наращивания e2e в форках."""

from __future__ import annotations

from client.http import Api
from client.models import UserView


def get_user(api: Api, user_id: str) -> UserView:
    r = api.expect("GET", f"/api/users/{user_id}", expected=200)
    return UserView.model_validate(r.json()["data"])
