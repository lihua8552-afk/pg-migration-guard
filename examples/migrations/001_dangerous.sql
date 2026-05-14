BEGIN;

CREATE INDEX CONCURRENTLY idx_orders_user_id ON orders (user_id);

ALTER TABLE users
  ADD COLUMN plan text NOT NULL DEFAULT 'free',
  DROP COLUMN legacy_status;

UPDATE accounts SET active = false;

DROP TABLE old_events;

COMMIT;
