import pytest

from client.http import Api


@pytest.mark.smoke
def test_openapi_served(stack: str) -> None:
    assert Api(stack).status("GET", "/openapi.yaml") == 200
