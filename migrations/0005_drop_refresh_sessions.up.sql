-- infra-handoff: прежние refresh-сессии удаляются — после выкатки все пользователи входят заново один раз; сессии — authn_sessions кита authnkit/session (sources.Kits в cmd/migrate)
DROP TABLE refresh_sessions;
