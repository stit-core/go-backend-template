"""Типизированная httpx-обёртка над API.

Один экземпляр `Api` = одна cookie-сессия (свой jar), как отдельный
cookie-jar в bash-смоке. Для replay-сценариев заводи второй `Api`.
"""

from __future__ import annotations

from types import TracebackType

import httpx

from client.models import (
    AuthUserView,
    LoginRequest,
    LoginResponse,
    MeResponse,
    RegisterRequest,
    RegisterResponse,
    UserUpdateRequest,
    UserUpdateResponse,
    UserView,
)


class UnexpectedStatus(AssertionError):
    def __init__(
        self, method: str, path: str, expected: int, got: int, body: str
    ) -> None:
        super().__init__(
            f"{method} {path}: expected {expected}, got {got}; body: {body[:500]}"
        )
        self.expected = expected
        self.got = got


class Api:
    def __init__(self, base_url: str, *, timeout: float = 10.0) -> None:
        self._c = httpx.Client(base_url=base_url, timeout=timeout)

    def __enter__(self) -> Api:
        return self

    def __exit__(
        self,
        exc_type: type[BaseException] | None,
        exc: BaseException | None,
        tb: TracebackType | None,
    ) -> None:
        self._c.close()

    def close(self) -> None:
        self._c.close()

    # --- cookie helpers (replay/rotation сценарии) ---

    @property
    def cookies(self) -> httpx.Cookies:
        return self._c.cookies

    def refresh_cookie(self) -> str | None:
        return self._c.cookies.get("refresh_token")

    def set_refresh_cookie(self, value: str) -> None:
        self._c.cookies.set("refresh_token", value)

    # --- low-level ---

    def expect(
        self,
        method: str,
        path: str,
        *,
        expected: int,
        json: object | None = None,
    ) -> httpx.Response:
        r = self._c.request(method, path, json=json)
        if r.status_code != expected:
            raise UnexpectedStatus(method, path, expected, r.status_code, r.text)
        return r

    def status(self, method: str, path: str, *, json: object | None = None) -> int:
        """Сырой код без проверки — для негативных кейсов."""
        return self._c.request(method, path, json=json).status_code

    # --- typed ops (контракт = delivery/http Go-структуры) ---

    def register(self, req: RegisterRequest) -> UserView:
        r = self.expect(
            "POST", "/api/auth/register", expected=201, json=req.model_dump()
        )
        return RegisterResponse.model_validate_json(r.content).data

    def login(self, req: LoginRequest) -> AuthUserView:
        r = self.expect("POST", "/api/auth/login", expected=200, json=req.model_dump())
        return LoginResponse.model_validate_json(r.content).user

    def me(self) -> UserView:
        r = self.expect("GET", "/api/users/me", expected=200)
        return MeResponse.model_validate_json(r.content).data

    def update_user(self, user_id: str, req: UserUpdateRequest) -> UserView:
        r = self.expect(
            "PATCH",
            f"/api/users/{user_id}",
            expected=200,
            json=req.model_dump(exclude_none=True),
        )
        return UserUpdateResponse.model_validate_json(r.content).data

    def refresh(self) -> None:
        self.expect("POST", "/api/auth/refresh", expected=204)

    def logout(self) -> None:
        self.expect("POST", "/api/auth/logout", expected=204)
