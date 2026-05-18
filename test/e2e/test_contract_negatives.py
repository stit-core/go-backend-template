"""Тонкие негативы: schema-валидация (huma → 422), 404, 401-без-cookie."""

import uuid

import pytest

from client.http import Api


@pytest.mark.e2e
def test_register_validation_rejected(api: Api) -> None:
    # huma валидирует request-схему до домена → 4xx (422 у huma v2;
    # допускаем 400 на случай иной конфигурации error-статуса).
    assert api.status(
        "POST", "/api/auth/register",
        json={"email": "not-an-email", "password": "short", "name": ""},
    ) in (400, 422)


@pytest.mark.e2e
def test_get_unknown_user_404(admin: Api) -> None:
    assert admin.status("GET", f"/api/users/{uuid.uuid4()}") == 404


@pytest.mark.e2e
def test_protected_without_cookie_401(api: Api) -> None:
    assert api.status("GET", "/api/users/me") == 401
    assert api.status("GET", "/api/users") == 401
