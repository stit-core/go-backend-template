"""Переиспользуемые step-функции auth-флоу над Api. Без ассертов — возвращают
значения, проверяет их вызывающий тест.
"""

from __future__ import annotations

from client.http import Api
from client.models import LoginRequest


def login(api: Api, email: str, password: str) -> None:
    api.login(LoginRequest(email=email, password=password))


def rotate_refresh(api: Api) -> tuple[str | None, str | None]:
    """refresh() с захватом refresh-cookie до и после ротации."""
    old = api.refresh_cookie()
    api.refresh()
    return old, api.refresh_cookie()


def replay_refresh_status(replay_api: Api, refresh_value: str) -> int:
    """Код ответа на /refresh с подсунутым (отозванным) refresh-cookie."""
    replay_api.set_refresh_cookie(refresh_value)
    return replay_api.status("POST", "/api/auth/refresh")
