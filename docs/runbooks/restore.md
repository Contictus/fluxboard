# PostgreSQL restore runbook

This is a manual restore procedure for a local or recovery environment. The
repository does not currently provision an automated backup sidecar, so a dump
must exist before this procedure can be used.

## Restore into a disposable database

Use a database that is not serving application traffic. Replace the file path
with the verified dump location.

```powershell
docker compose -f deploy/docker-compose.yml exec -T postgres createdb -U postgres fluxboard_restore
Get-Content .\backups\fluxboard.dump -Raw | docker compose -f deploy/docker-compose.yml exec -T postgres pg_restore -U postgres -d fluxboard_restore --clean --if-exists
```

Verify the migration history and representative tenant rows before switching
traffic. Do not restore over the live database as a first validation step.

```powershell
docker compose -f deploy/docker-compose.yml exec postgres psql -U postgres -d fluxboard_restore -c "SELECT version, dirty FROM schema_migrations;"
docker compose -f deploy/docker-compose.yml exec postgres psql -U postgres -d fluxboard_restore -c "SELECT count(*) FROM organizations;"
```

The restored database must be brought to the expected migration version before
the application is started against it. Keep the owner/app role separation and
verify RLS policies after restore; a superuser query alone does not validate
tenant isolation.
