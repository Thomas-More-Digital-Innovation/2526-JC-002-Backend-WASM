-- Statuses lookup table
CREATE TABLE IF NOT EXISTS statuses (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT    NOT NULL UNIQUE
);

-- Seed default statuses
INSERT OR IGNORE INTO statuses (name) VALUES ('todo');
INSERT OR IGNORE INTO statuses (name) VALUES ('in_progress');
INSERT OR IGNORE INTO statuses (name) VALUES ('done');

-- Todos table
CREATE TABLE IF NOT EXISTS todos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    title       TEXT    NOT NULL,
    description TEXT    NOT NULL UNIQUE,
    status_id   INTEGER NOT NULL REFERENCES statuses(id) ON DELETE RESTRICT
);
