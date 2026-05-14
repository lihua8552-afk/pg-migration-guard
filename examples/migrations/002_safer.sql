CREATE INDEX CONCURRENTLY idx_orders_created_at ON orders (created_at);

ALTER TABLE users
  ADD COLUMN plan text;
