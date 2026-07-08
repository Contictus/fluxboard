DROP TABLE IF EXISTS entitlement_overrides;
DROP TABLE IF EXISTS feature_flags;
DROP TABLE IF EXISTS api_keys;
ALTER TABLE plans DROP COLUMN IF EXISTS monthly_price;
