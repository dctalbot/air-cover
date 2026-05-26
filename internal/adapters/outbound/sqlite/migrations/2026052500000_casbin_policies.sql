-- +goose Up
CREATE TABLE IF NOT EXISTS casbin_rule (
    p_type VARCHAR(32) DEFAULT '' NOT NULL,
    v0 VARCHAR(255) DEFAULT '' NOT NULL,
    v1 VARCHAR(255) DEFAULT '' NOT NULL,
    v2 VARCHAR(255) DEFAULT '' NOT NULL,
    v3 VARCHAR(255) DEFAULT '' NOT NULL,
    v4 VARCHAR(255) DEFAULT '' NOT NULL,
    v5 VARCHAR(255) DEFAULT '' NOT NULL,
    CHECK (TYPEOF("p_type") = "text" AND LENGTH("p_type") <= 32),
    CHECK (TYPEOF("v0") = "text" AND LENGTH("v0") <= 255),
    CHECK (TYPEOF("v1") = "text" AND LENGTH("v1") <= 255),
    CHECK (TYPEOF("v2") = "text" AND LENGTH("v2") <= 255),
    CHECK (TYPEOF("v3") = "text" AND LENGTH("v3") <= 255),
    CHECK (TYPEOF("v4") = "text" AND LENGTH("v4") <= 255),
    CHECK (TYPEOF("v5") = "text" AND LENGTH("v5") <= 255)
);

CREATE INDEX IF NOT EXISTS idx_casbin_rule ON casbin_rule (p_type, v0, v1);

-- +goose Down
DROP INDEX IF EXISTS idx_casbin_rule;
DROP TABLE IF EXISTS casbin_rule;
