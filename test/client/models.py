"""Pydantic-зеркало wire-DTO huma (internal/domain/{auth,user}/delivery/http).

Поля и их json-имена — те же, что у *Input/*Output/*View Go-структур: лишнее поле и тип не того
вида — ошибка разбора, а не молчаливое приведение.
"""

from datetime import datetime

from pydantic import BaseModel, ConfigDict, Field


class Wire(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True)


class Body(Wire):
    """Тело ответа: huma добавляет ссылку на схему тела — `$schema`."""

    schema_: str | None = Field(default=None, alias="$schema")


class RegisterRequest(Wire):
    email: str
    password: str
    name: str


class LoginRequest(Wire):
    email: str
    password: str


class ChangePasswordRequest(Wire):
    user_id: str
    old_password: str
    new_password: str


class UserUpdateRequest(Wire):
    name: str | None = None
    status: str | None = None
    version: int


class UserView(Wire):
    id: str
    email: str
    name: str
    status: str
    version: int
    created_at: datetime
    updated_at: datetime


class AuthUserView(Wire):
    id: str
    email: str
    name: str
    role: str


class UserRegisterResponse(Body):
    data: UserView


class UserMeResponse(Body):
    data: UserView


class UserGetResponse(Body):
    data: UserView


class UserUpdateResponse(Body):
    data: UserView


class AuthLoginResponse(Body):
    data: AuthUserView
