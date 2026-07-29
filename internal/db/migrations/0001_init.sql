-- +goose Up
-- +goose StatementBegin

-- Enum-like columns (kind, status) are plain TEXT; valid values are enforced in
-- the app, not the DB. Geo uses geography(Point,4326) so distances come back in
-- meters. Embedding dim (768) is tied to Ollama's nomic-embed-text: changing the
-- model means changing the column and re-embedding.

CREATE EXTENSION IF NOT EXISTS postgis;
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL UNIQUE,
    first_name  TEXT NOT NULL,
    middle_name TEXT,
    last_name   TEXT NOT NULL,
    phone       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE magic_links (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash BYTEA NOT NULL, -- hash only; the raw token is emailed, never stored
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_magic_links_user ON magic_links (user_id);

CREATE TABLE reports (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL, -- 'lost' | 'found'
    category    TEXT NOT NULL,
    description TEXT NOT NULL,
    description_embedding VECTOR(768),
    location    GEOGRAPHY(Point, 4326),
    event_at    TIMESTAMPTZ, -- when the item was lost/found, distinct from created_at
    status      TEXT NOT NULL DEFAULT 'active', -- active | matched | resolved | expired
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_reports_pool ON reports (kind, category, status);
CREATE INDEX idx_reports_location ON reports USING GIST (location);
-- HNSW cosine index for semantic search; requires pgvector >= 0.5.
CREATE INDEX idx_reports_embedding ON reports
    USING hnsw (description_embedding vector_cosine_ops);
CREATE INDEX idx_reports_user_created ON reports (user_id, created_at);
CREATE INDEX idx_reports_expires ON reports (expires_at) WHERE status = 'active';

-- Finder-only private details, AES-GCM encrypted. The key lives in app config,
-- not the database, and plaintext is never persisted or returned to the loser.
CREATE TABLE verification_details (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    report_id  UUID NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    kind       TEXT NOT NULL, -- e.g. 'amount', 'location_detail', 'brand', 'contents'
    ciphertext BYTEA NOT NULL,
    nonce      BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_verification_details_report ON verification_details (report_id);

CREATE TABLE matches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lost_report_id  UUID NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    found_report_id UUID NOT NULL REFERENCES reports(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending_verify', -- pending_verify | verified | resolved | discarded
    confidence      REAL,
    attempts_used   INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (lost_report_id, found_report_id)
);

CREATE INDEX idx_matches_lost  ON matches (lost_report_id);
CREATE INDEX idx_matches_found ON matches (found_report_id);

CREATE TABLE verification_questions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id   UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    prompt     TEXT NOT NULL,
    detail_id  UUID REFERENCES verification_details(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_verification_questions_match ON verification_questions (match_id);

CREATE TABLE verification_answers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID NOT NULL REFERENCES verification_questions(id) ON DELETE CASCADE,
    answer_text TEXT NOT NULL,
    score       REAL, -- Ollama-graded [0,1], combined into matches.confidence
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_verification_answers_question ON verification_answers (question_id);

CREATE TABLE messages (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id       UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    sender_user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body           TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_messages_match ON messages (match_id, created_at);

-- In-app notifications only; email is sent, not stored. entry_type/entry_id point
-- at what the notification is about (e.g. 'match', 'report') for deep-linking.
CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entry_type TEXT NOT NULL,
    entry_id   UUID NOT NULL,
    title      TEXT NOT NULL,
    message    TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at    TIMESTAMPTZ
);

CREATE INDEX idx_notifications_user ON notifications (user_id, created_at);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS verification_answers;
DROP TABLE IF EXISTS verification_questions;
DROP TABLE IF EXISTS matches;
DROP TABLE IF EXISTS verification_details;
DROP TABLE IF EXISTS reports;
DROP TABLE IF EXISTS magic_links;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
