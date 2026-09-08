# Local development runbook

This runbook describes the supported local sequence for the Docker Compose
stack. Run commands from the repository root.

## First run

```powershell
Copy-Item .env.example .env
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.override.yml up --build -d
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.override.yml ps
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.override.yml exec api sh
```

Apply migrations from a separate terminal with `make migrate`. The API and web
development servers are exposed at `http://localhost:8080` and
`http://localhost:3000`; Mailpit is at `http://localhost:8025`.

## Verification

```powershell
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:8080/readyz
docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.override.yml ps
```

`healthz` only proves that the API process is alive. `readyz` also checks the
runtime dependencies and is the meaningful local readiness check.

## Stop and reset

`make down` stops the stack and removes Compose volumes, including local
PostgreSQL, Redis, and MinIO data. Use `docker compose ... stop` when the data
must be retained and the stack will be resumed later.
