-- +goose Up
CREATE TYPE node_status AS ENUM ('enrolling', 'online', 'offline', 'removed');

-- Exactly one row: the cluster this control center manages.
CREATE TABLE cluster (
  id          uuid PRIMARY KEY,
  name        text NOT NULL,
  ca_cert     bytea NOT NULL,
  cmd_key_id  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  singleton   boolean NOT NULL DEFAULT true UNIQUE CHECK (singleton)
);

-- Private keys and other secrets, AES-256-GCM with the master key.
CREATE TABLE secrets (
  id           uuid PRIMARY KEY,
  scope        text NOT NULL,
  name         text NOT NULL,
  ciphertext   bytea NOT NULL,
  nonce        bytea NOT NULL,
  key_version  int NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  rotated_at   timestamptz,
  UNIQUE (scope, name)
);

-- Join codes. Only the HKDF-derived key is stored, never the code.
CREATE TABLE enrollment_codes (
  id            uuid PRIMARY KEY,
  purpose       text NOT NULL CHECK (purpose IN ('join')),
  code_key      bytea NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  expires_at    timestamptz NOT NULL,
  used_at       timestamptz,
  used_by_node  uuid
);
CREATE INDEX enrollment_codes_active ON enrollment_codes (expires_at) WHERE used_at IS NULL;

CREATE TABLE nodes (
  id              uuid PRIMARY KEY,
  name            text NOT NULL,
  hostname        text,
  status          node_status NOT NULL,
  os              jsonb,
  hardware        jsonb,
  capabilities    text[] NOT NULL DEFAULT '{}',
  agent_version   text,
  cert_serial     text NOT NULL,
  cert_not_after  timestamptz NOT NULL,
  ip_addresses    inet[] NOT NULL DEFAULT '{}',
  mac_addresses   macaddr[] NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now(),
  last_seen_at    timestamptz,
  removed_at      timestamptz
);

-- Raw samples every 5 s, daily partitions, kept 48 h.
CREATE TABLE node_stats_raw (
  node_id            uuid NOT NULL,
  ts                 timestamptz NOT NULL,
  cpu_pct            real,
  load1              real,
  mem_used           bigint,
  mem_total          bigint,
  swap_used          bigint,
  disk_used_pct_max  real,
  net_rx_bps         bigint,
  net_tx_bps         bigint,
  cpu_temp_c         real,
  gpu_util_pct       real,
  on_battery         boolean,
  detail             jsonb,
  PRIMARY KEY (node_id, ts)
) PARTITION BY RANGE (ts);

-- 5-minute rollups, daily partitions, kept 90 days.
CREATE TABLE node_stats_5m (
  node_id            uuid NOT NULL,
  bucket             timestamptz NOT NULL,
  cpu_pct_avg        real,
  cpu_pct_max        real,
  mem_used_avg       bigint,
  mem_used_max       bigint,
  mem_total          bigint,
  disk_used_pct_max  real,
  net_rx_bps_avg     bigint,
  net_tx_bps_avg     bigint,
  cpu_temp_c_max     real,
  gpu_util_pct_avg   real,
  PRIMARY KEY (node_id, bucket)
) PARTITION BY RANGE (bucket);

-- +goose Down
DROP TABLE node_stats_5m;
DROP TABLE node_stats_raw;
DROP TABLE nodes;
DROP TABLE enrollment_codes;
DROP TABLE secrets;
DROP TABLE cluster;
DROP TYPE node_status;
