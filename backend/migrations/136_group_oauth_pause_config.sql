CREATE TABLE IF NOT EXISTS group_oauth_pause_configs (
    group_id BIGINT PRIMARY KEY REFERENCES groups(id) ON DELETE CASCADE,
    oauth_5h_pause_percent DECIMAL(20,8),
    oauth_5h_pause_amount_usd DECIMAL(20,8),
    oauth_7d_pause_percent DECIMAL(20,8),
    oauth_7d_pause_amount_usd DECIMAL(20,8),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
