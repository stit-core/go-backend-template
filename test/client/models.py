"""Pydantic-зеркало wire-DTO huma (internal/domain/{auth,user}/delivery/http).

Поля и их json-имена держать в синхроне с *Input/*Output/*View Go-структур.
"""

from datetime import datetime

from pydantic import BaseModel


class RegisterRequest(BaseModel):
    email: str
    password: str
    name: str


class LoginRequest(BaseModel):
    email: str
    password: str


class ChangePasswordRequest(BaseModel):
    user_id: str
    old_password: str
    new_password: str


class UserView(BaseModel):
    id: str
    email: str
    name: str
    status: str
    created_at: datetime
    updated_at: datetime


class AuthUserView(BaseModel):
    id: str
    email: str
    name: str
    role: str


class RegisterResponse(BaseModel):
    data: UserView


class MeResponse(BaseModel):
    data: UserView


class LoginResponse(BaseModel):
    user: AuthUserView
