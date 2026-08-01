-- 001_init.sql
-- Delivery ledger: one row per source message (or media group) per
-- destination platform. The unique constraint is what prevents duplicate
-- forwarding after retries and restarts.

CREATE TABLE IF NOT EXISTS deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_platform TEXT NOT NULL,
    source_chat_id INTEGER NOT NULL,
    source_key TEXT NOT NULL,
    destination_platform TEXT NOT NULL,
    destination_message_id INTEGER,
    status TEXT NOT NULL,
    error_message TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,

    UNIQUE (
        source_platform,
        source_chat_id,
        source_key,
        destination_platform
    )
);

CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
