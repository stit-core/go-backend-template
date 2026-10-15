"""Pydantic-зеркало wire-DTO huma (internal/domain/{auth,user}/delivery/http).

Поля и их json-имена — те же, что у *Input/*Output/*View Go-структур: лишнее поле и тип не того
вида — ошибка разбора, а не молчаливое приведение.
"""

from datetime import datetime

from pydantic import BaseModel, ConfigDict


class Wire(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)


class RegisterRequest(Wire):
    email: str
    password: str
    name: str


class LoginRequest(Wire):
    email: str
    password: str


class ChangePasswordRequest(Wire):
    old_password: str
    new_password: str


class UserUpdateRequest(Wire):
    name: str | None = None
    status: str | None = None
    version: int


class UserView(Wire):
    id: str
    email: str
    email_verified: bool
    name: str
    role: str
    status: str
    version: int
    created_at: datetime
    updated_at: datetime


class AuthUserView(Wire):
    id: str
    email: str
    name: str
    role: str


class UserRegisterResponse(Wire):
    data: UserView


class UserMeResponse(Wire):
    data: UserView


class UserGetResponse(Wire):
    data: UserView


class UserUpdateResponse(Wire):
    data: UserView


class AuthLoginResponse(Wire):
    data: AuthUserView
