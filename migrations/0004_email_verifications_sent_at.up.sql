-- Повтор доставки user.events.registered не теряет письмо: отправленное (sent_at) пропускается,
-- неотправленное после сбоя mailer перевыпускается с новым токеном.
ALTER TABLE email_verifications ADD COLUMN sent_at TIMESTAMPTZ;
