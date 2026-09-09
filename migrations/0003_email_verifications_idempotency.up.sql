-- At-least-once consumer (user.events.registered) must not create a second
-- pending verification per user: redelivery hits ON CONFLICT and becomes a no-op.
CREATE UNIQUE INDEX email_verifications_pending_user_idx
    ON email_verifications (user_id)
    WHERE used_at IS NULL;
