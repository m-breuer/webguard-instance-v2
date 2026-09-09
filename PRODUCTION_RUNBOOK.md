# Production WebGuard Instance Runbook

This runbook documents the current production worker at `de-1.webguard.dev`.
It communicates with WebGuard Core at `https://app.webguard.dev` through the
instance API at `https://app.webguard.dev/api/instances`.

## Production configuration

Configure these values in the deployment environment. Keep the API key in the
deployment secret store; never commit it or include it in logs.

```dotenv
WEBGUARD_LOCATION=de-1
WEBGUARD_INSTANCE_ID=de-1-worker-1
WEBGUARD_CORE_API_URL=https://app.webguard.dev
WEBGUARD_INSTANCE_API_BASE_PATH=/api/instances
WEBGUARD_CORE_API_KEY=<deployment-secret>
```

`WEBGUARD_LOCATION` must match the Core instance code. `WEBGUARD_INSTANCE_ID`
is a stable, unique worker identity and must not be reused by another worker.
The example identity is illustrative; use the actual stable identity assigned
to the deployment.

## Docker and Coolify routing

The production image is built from the `production` target and listens on
container port `8080` by default. With Docker Compose:

```bash
docker compose -f compose.yml up -d --build
```

For Coolify, route `de-1.webguard.dev` to the WebGuard Instance service on
container port `8080`. Store all environment values in the deployment's
environment configuration, with `WEBGUARD_CORE_API_KEY` marked as a secret.
Do not publish the Core API key, or use it as a URL or health-check value.

## Health checks

Run these checks against the deployment after startup or an upgrade:

```bash
curl --fail https://de-1.webguard.dev/livez
curl --fail https://de-1.webguard.dev/readyz
curl --fail https://de-1.webguard.dev/metrics
```

`/livez` confirms that the process accepts HTTP requests. `/readyz` confirms
that Core configuration is complete and the worker is not draining. `/metrics`
returns Prometheus text metrics without monitoring targets, credentials, or
job identifiers.

## Upgrade

1. Update the image tag or checkout to the intended release.
2. Verify that the environment still contains the production values above.
3. Deploy the new image:

   ```bash
   docker compose -f compose.yml up -d --build
   ```

4. Verify `/livez`, `/readyz`, `/metrics`, and the worker logs.
5. Confirm in Core that the `de-1` instance is reporting normally.

For Coolify, deploy the selected image or commit through the service's normal
deployment workflow, then perform the same health and Core checks.

## Rollback

1. Select the last known-good image tag or commit.
2. Re-deploy that version without changing the production secrets.
3. Verify `/livez`, `/readyz`, `/metrics`, and Core reporting again.
4. Record the failed version and the rollback reason in the deployment log.

If readiness remains unhealthy, inspect the resolved environment values and the
Core API response first. Never paste API keys into incident reports or command
output shared outside the deployment environment.
