-- Reverse 0011 in dependency order (children before parents; subscriptions +
-- invoices + usage_records + outbox reference plans/organizations).
DROP TABLE IF EXISTS outbox;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS processed_stripe_events;
DROP TABLE IF EXISTS invoices;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS plans;
