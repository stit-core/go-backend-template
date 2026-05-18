"""E2E showcase: register → outbox-строка → core-NATS доставка →
verification consume → verification.events.email_verification_requested +
строка email_verifications. Полный async-путь через инфру.
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

USER_REGISTERED = "user.events.registered"
EMAIL_VERIF_REQUESTED = "verification.events.email_verification_requested"


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

        # 1. outbox: строка user.events.registered (детерминированно), затем дрен
        row = outbox.wait_for(
            pg, USER_REGISTERED, predicate=lambda p: p.get("Email") == email
        )
        assert row["payload"]["ID"] == uid
        outbox.wait_for(
            pg,
            USER_REGISTERED,
            predicate=lambda p: p.get("Email") == email,
            published=True,
        )

        # 2. core-NATS: реальная доставка user.events.registered
        ev = nc.wait_for(
            USER_REGISTERED, predicate=lambda p: p.get("Email") == email, timeout=10
        )
        assert ev["ID"] == uid

        # 3. cross-domain: verification consume → emitted email_verification_requested
        nc.wait_for(EMAIL_VERIF_REQUESTED, timeout=10)

    # 4. verification side-effect персистнут: строка email_verifications
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if pg.email_verifications_for(uid):
            break
        time.sleep(0.2)
    assert pg.email_verifications_for(uid), "email_verifications row не появилась"
