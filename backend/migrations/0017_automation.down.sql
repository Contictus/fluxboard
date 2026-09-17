-- 0017_automation rollback.
DROP INDEX IF EXISTS automation_rules_org_idx;
DROP TABLE IF EXISTS automation_rules;
