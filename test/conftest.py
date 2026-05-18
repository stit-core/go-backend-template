"""pytest-фикстуры: session-стек (shell-out в stack/up.sh|down.sh) + api + user."""

from __future__ import annotations

import os
import re
import subprocess
import uuid
from collections.abc import Callable, Iterator
from dataclasses import dataclass
from pathlib import Path

import pytest

from client.http import Api
from client.models import LoginRequest, RegisterRequest
from infra.nats import NatsCollector
from infra.pg import Pg
from infra.redis import RedisInspect

_REPO_ROOT = Path(__file__).resolve().parents[1]
_ENDPOINTS_FILE = _REPO_ROOT / "test" / "stack" / ".e2e-endpoints.env"


def _read_endpoints() -> dict[str, str]:
    """Резолвнутые эфемерные адреса стека — пишет test/stack/up.sh после
    `compose up` (порты эфемерны для изоляции параллельных форков)."""
    eps: dict[str, str] = {}
    for line in _ENDPOINTS_FILE.read_text().splitlines():
        s = line.strip()
        if not s or s.startswith("#") or "=" not in s:
            continue
        k, _, v = s.partition("=")
        eps[k.strip()] = v.strip().strip("\"'")
    return eps


@dataclass(frozen=True)
class RegisteredUser:
    email: str
    password: str
    name: str


def _env_local(key: str, default: str) -> str:
    """env > ./.env.local > default. Значение не логируем (как up.sh)."""
    if os.environ.get(key):
        return os.environ[key]
    f = _REPO_ROOT / ".env.local"
    if f.is_file():
        pat = re.compile(rf"^\s*(?:export\s+)?{re.escape(key)}=(.*)$")
        for line in f.read_text().splitlines():
            m = pat.match(line)
            if m:
                return m.group(1).strip().strip("\"'")
    return default


def _run(script: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["bash", f"test/stack/{script}"],
        cwd=_REPO_ROOT,
        capture_output=True,
        text=True,
        check=False,
    )


@pytest.fixture(scope="session")
def stack() -> Iterator[str]:
    up = _run("up.sh")
    if up.returncode != 0:
        down = _run("down.sh")
        pytest.fail(
            "stack up.sh failed:\n"
            f"--- stdout ---\n{up.stdout}\n--- stderr ---\n{up.stderr}\n"
            f"--- down stdout ---\n{down.stdout}",
            pytrace=False,
        )
    try:
        yield _read_endpoints()["E2E_API_BASE_URL"]
    finally:
        _run("down.sh")


@pytest.fixture
def api(stack: str) -> Iterator[Api]:
    with Api(stack) as client:
        yield client


@pytest.fixture
def user(api: Api) -> RegisteredUser:
    u = RegisteredUser(
        email=f"e2e-{uuid.uuid4().hex[:12]}@example.com",
        password="e2e-pass-12345",
        name="E2E Tester",
    )
    api.register(RegisterRequest(email=u.email, password=u.password, name=u.name))
    return u


@pytest.fixture
def admin(stack: str) -> Iterator[Api]:
    """Api, залогиненный как admin. Самого админа создаёт cmd/seed-admin
    one-off при старте стека; здесь только login кредами из .env.local."""
    email = _env_local("E2E_ADMIN_EMAIL", "e2e-admin@example.com")
    password = _env_local("E2E_ADMIN_PASSWORD", "e2e-admin-pass-1234")
    with Api(stack) as client:
        client.login(LoginRequest(email=email, password=password))
        yield client


@pytest.fixture
def other_user(stack: str) -> RegisteredUser:
    """Второй обычный client (cross-user 403). Регистрируется своим Api,
    чтобы не трогать cookie-сессию теста."""
    u = RegisteredUser(
        email=f"other-{uuid.uuid4().hex[:12]}@example.com",
        password="e2e-pass-12345",
        name="Other User",
    )
    with Api(stack) as c:
        c.register(RegisterRequest(email=u.email, password=u.password, name=u.name))
    return u


@pytest.fixture(scope="session")
def endpoints(stack: str) -> dict[str, str]:
    return _read_endpoints()


@pytest.fixture
def pg(endpoints: dict[str, str]) -> Iterator[Pg]:
    with Pg(dsn=endpoints["E2E_PG_DSN"]) as p:
        yield p


@pytest.fixture
def redis(endpoints: dict[str, str]) -> Iterator[RedisInspect]:
    with RedisInspect(url=endpoints["E2E_REDIS_URL"]) as r:
        yield r


@pytest.fixture
def nats_collector(
    endpoints: dict[str, str],
) -> Callable[[list[str]], NatsCollector]:
    """Фабрика: тест сам входит в `with`, чтобы подписка встала ДО HTTP-действия
    (core NATS без реплея)."""

    def _make(subjects: list[str]) -> NatsCollector:
        return NatsCollector(subjects, url=endpoints["E2E_NATS_URL"])

    return _make
