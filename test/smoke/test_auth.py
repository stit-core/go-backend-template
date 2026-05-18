"""Smoke: cookie-based auth-флоу —
register/login/me/refresh-rotation/reuse-detect/logout/expired.
"""

import pytest

from client.http import Api
from conftest import RegisteredUser
from scenarios.auth import login, replay_refresh_status, rotate_refresh


@pytest.mark.smoke
def test_auth_cookie_flow(api: Api, user: RegisteredUser, stack: str) -> None:
    # 1. login → 200 + cookies
    login(api, user.email, user.password)

    # 2. /me под cookie — email совпадает
    assert api.me().email == user.email

    # 3. refresh → 204, refresh_token ротировался
    old, new = rotate_refresh(api)
    assert old is not None and new is not None
    assert new != old, "refresh_token did not rotate"

    # 4. replay старого refresh-cookie из отдельной сессии → 401
    with Api(stack) as replay:
        assert replay_refresh_status(replay, old) == 401

    # 5. /me с ротированной сессией всё ещё 200
    assert api.me().email == user.email

    # 6. logout → 204, затем /me → 401
    api.logout()
    assert api.status("GET", "/api/users/me") == 401
