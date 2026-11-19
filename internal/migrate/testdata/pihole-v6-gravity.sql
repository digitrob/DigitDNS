-- Reduced Pi-hole v6 gravity.db schema used by the importer fixtures.
-- This is not a full Pi-hole database: it is the subset the importer reads.
-- Regenerate pihole-v6-gravity.db from this file with:
--   sqlite3 pihole-v6-gravity.db < pihole-v6-gravity.sql

CREATE TABLE gravity (
  domain TEXT NOT NULL,
  adlist_id INTEGER NOT NULL,
  PRIMARY KEY (domain, adlist_id)
);

CREATE TABLE adlist (
  id INTEGER PRIMARY KEY,
  address TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  comment TEXT,
  date_added INTEGER,
  date_modified INTEGER
);

CREATE TABLE domainlist (
  id INTEGER PRIMARY KEY,
  type INTEGER NOT NULL,
  domain TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  date_added INTEGER,
  date_modified INTEGER,
  comment TEXT
);

CREATE TABLE client (
  id INTEGER PRIMARY KEY,
  ip TEXT NOT NULL,
  name TEXT,
  comment TEXT
);

CREATE TABLE "group" (
  id INTEGER PRIMARY KEY,
  name TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE domainlist_by_group (
  domainlist_id INTEGER NOT NULL,
  group_id INTEGER NOT NULL,
  PRIMARY KEY (domainlist_id, group_id)
);

CREATE TABLE client_by_group (
  client_id INTEGER NOT NULL,
  group_id INTEGER NOT NULL,
  PRIMARY KEY (client_id, group_id)
);

CREATE TABLE info (
  property TEXT PRIMARY KEY,
  value TEXT
);

INSERT INTO adlist (id, address, enabled, comment) VALUES
  (1, 'https://lists.example.invalid/ads.txt', 1, 'Synthetic ads'),
  (2, 'https://lists.example.invalid/trackers.txt', 1, 'Synthetic trackers');

INSERT INTO gravity (domain, adlist_id) VALUES
  ('ads.example', 1),
  ('tracker.example', 1),
  ('telemetry.example', 2);

INSERT INTO domainlist (id, type, domain, enabled, comment) VALUES
  (1, 0, 'allowed.example', 1, 'Synthetic allow'),
  (2, 1, 'blocked.example', 1, 'Synthetic block'),
  (3, 2, '^safe[0-9]+\\.example$', 1, 'Synthetic regex allow'),
  (4, 3, '^ad[0-9]+\\.example$', 1, 'Synthetic regex block');

INSERT INTO client (id, ip, name, comment) VALUES
  (1, '192.0.2.40', 'desktop', 'Synthetic desktop'),
  (2, '192.0.2.41', 'phone', 'Synthetic phone');

INSERT INTO "group" (id, name, enabled) VALUES
  (1, 'Default', 1);

INSERT INTO domainlist_by_group (domainlist_id, group_id) VALUES
  (1, 1), (2, 1), (3, 1), (4, 1);

INSERT INTO client_by_group (client_id, group_id) VALUES
  (1, 1), (2, 1);

INSERT INTO info (property, value) VALUES
  ('version', 'v6.0-synthetic');
