-- Store incoming WhatsApp attachments in Supabase Postgres instead of local disk.
CREATE TABLE IF NOT EXISTS whatsapp.api_media (
  id TEXT PRIMARY KEY,
  session_id TEXT NOT NULL,
  payload BYTEA NOT NULL,
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  filename TEXT NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_api_media_session
  ON whatsapp.api_media(session_id, created_at);

ALTER TABLE whatsapp.api_media ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON TABLE whatsapp.api_media FROM PUBLIC, anon, authenticated, service_role;
