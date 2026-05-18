"""pytest-фикстуры: session-стек (shell-out в stack/up.sh|down.sh) + api + user."""

from __future__ import annotations

import subprocess
import uuid
from collections.abc import Iterator
from dataclasses import dataclass
from pathlib import Path

import pytest

from client.http import Api
from client.models import RegisterRequest

_REPO_ROOT = Path(__file__).resolve().parents[1]
_BASE_URL = "http://127.0.0.1:8080"


@dataclass(frozen=True)
class RegisteredUser:
    email: str
    password: str
    name: str


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
        yield _BASE_URL
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
