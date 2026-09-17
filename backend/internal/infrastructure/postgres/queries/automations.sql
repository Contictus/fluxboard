-- Automation rules ([T], tenant-scoped, FR-AUTO) --------------------------

-- name: CreateAutomationRule :exec
INSERT INTO automation_rules (id, org_id, name, enabled, trigger, trigger_config, action, action_config, created_by)
VALUES (@id, @org_id, @name, @enabled, @trigger, @trigger_config, @action, @action_config, @created_by);

-- name: GetAutomationRule :one
SELECT id, org_id, name, enabled, trigger, trigger_config, action, action_config, created_by, created_at
FROM automation_rules
WHERE org_id = @org_id AND id = @id;

-- name: ListAutomationRules :many
SELECT id, org_id, name, enabled, trigger, trigger_config, action, action_config, created_by, created_at
FROM automation_rules
WHERE org_id = @org_id
ORDER BY created_at;

-- name: ListEnabledAutomationRules :many
SELECT id, org_id, name, enabled, trigger, trigger_config, action, action_config, created_by, created_at
FROM automation_rules
WHERE org_id = @org_id AND enabled AND trigger = @trigger
ORDER BY created_at;

-- name: UpdateAutomationRule :execrows
UPDATE automation_rules
SET name = @name, enabled = @enabled, trigger = @trigger, trigger_config = @trigger_config,
    action = @action, action_config = @action_config
WHERE org_id = @org_id AND id = @id;

-- name: DeleteAutomationRule :execrows
DELETE FROM automation_rules
WHERE org_id = @org_id AND id = @id;
