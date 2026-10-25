-- The idea pile: thoughts worth keeping that aren't tasks. A deleted idea keeps its row, so undo
-- can bring it back.
CREATE TABLE ideas (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    area TEXT,
    raw_text TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    deleted_at TEXT
);
