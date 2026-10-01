"""Step-функции user-домена. Паттерн для наращивания e2e в форках."""

from __future__ import annotations

from client.http import Api
from client.models import UserView


def get_user(api: Api, user_id: str) -> UserView:
    return api.get_user(user_id)
