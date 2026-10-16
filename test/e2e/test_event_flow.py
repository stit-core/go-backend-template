"""E2E showcase: register → outbox-строка → core-NATS доставка →
verification consume → verification.events.email_verification_requested.v2 +
строка email_verifications. Полный async-путь через инфру; в payload — только ID.
"""

import time
import uuid
from collections.abc import Callable

import pytest

from client.http import Api
from client.models import RegisterRequest
from infra import outbox
from infra.nats import NatsCollector
from infra.pg import Pg

USER_REGISTERED = "user.events.registered.v2"
EMAIL_VERIF_REQUESTED = "verification.events.email_verification_requested.v2"


@pytest.mark.smoke
@pytest.mark.e2e
def test_register_event_flow(
    api: Api,
    pg: Pg,
    nats_collector: Callable[[list[str]], NatsCollector],
) -> None:
    email = f"evflow-{uuid.uuid4().hex[:10]}@example.com"

    # Подписка ДО register — core NATS без реплея.
    with nats_collector([USER_REGISTERED, EMAIL_VERIF_REQUESTED]) as nc:
        uid = api.register(
            RegisterRequest(email=email, password="evflow-pass-12345", name="EvFlow")
        ).id

        # 1. outbox: строка user.events.registered.v2 (детерминированно), затем дрен
        row = outbox.wait_for(pg, USER_REGISTERED, predicate=lambda p: p.get("ID") == uid)
        assert set(row["payload"]) == {"ID", "At"}, "в payload только ID и время"
        outbox.wait_for(
            pg,
            USER_REGISTERED,
            predicate=lambda p: p.get("ID") == uid,
            published=True,
        )

        # 2. core-NATS: реальная доставка user.events.registered.v2
        nc.wait_for(USER_REGISTERED, predicate=lambda p: p.get("ID") == uid, timeout=10)

        # 3. cross-domain: verification consume → emitted email_verification_requested.v2
        ev = nc.wait_for(
            EMAIL_VERIF_REQUESTED,
            predicate=lambda p: p.get("UserID") == uid,
            timeout=10,
        )
        assert set(ev) == {"TokenID", "UserID", "At"}, "в payload только ID и время"

    # 4. verification side-effect персистнут: строка email_verifications
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if pg.email_verifications_for(uid):
            break
        time.sleep(0.2)
    assert pg.email_verifications_for(uid), "email_verifications row не появилась"
