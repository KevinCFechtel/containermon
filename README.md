# ContainerMon

ContainerMon is a lightweight daemon that monitors container runtime state (Docker or Podman), stores container metadata in a local SQLite database, and exposes a small web UI and HTTP API for viewing and updating container information.

Key features
- Periodically inspects containers and stores/upserts their metadata into a local SQLite DB.
- Supports Docker and Podman backends:
- Optional remote hosts synchronization.
- Optional Diun-compatible webhook handling to update image digests and mark images as updated.
- Sends notifications for container health or status changes via Shoutrrr (configurable).

Get it from docker hub
- `docker pull kevincfechtel/containermon:latest`

Configuration (flags and environment variables):
- SOCKET_FILE_PATH / -socketPath — path to Docker/Podman socket.
- DB_PATH / -dbPath — path to SQLite DB file.
- ENABLE_DIUN_WEBHOOK / -enableDiunWebhook — enable Diun webhook endpoints.
- AGENT_TOKEN / -agentToken — token required for /json access (optional).
- DIUN_WEBHOOK_TOKEN / -diunWebhookToken — token for /webhook (optional).
- WEBUI_PASSWORD / -webUIPassword — protect the web UI with a password (optional).
- WEB_SESSION_EXPIRATION_TIME / -webSessionExpirationTime — session expiration time in minutes (default: 60).
- CRON_HOST_HEALTH_CONFIG / -cronHostHealthConfig — cron schedule for host health checks (default: "*/5 * * * *").
- CRON_CONTAINER_HEALTH_CONFIG / -cronContainerHealthConfig — cron schedule for container health checks (default: "*/5 * * * * *").
- CRON_REMOTE_CONFIG / -cronRemoteConfig — cron schedule for remote host synchronization (default: disabled).
- CONTAINER_ERROR_URL / -containerErrorUrl — URL to report on container errors (optional).
- ENABLE_DEBUGGING / -debug — enable debug logging output.
- ENABLE_MESSAGE_ON_STARTUP / -messageOnStartup — send a message on startup.
- HOST_HEALTH_CHECK_URL / -hostHealthCheckUrl — URL to report host health (optional).

Notes
- The UI uses simple HTML templates in [assets/layout.html](assets/layout.html) and [assets/login.html](assets/login.html).
- Remote host integration: set environment variables prefixed with `REMOTE_CONFIG_HOST_<N>` and optional `REMOTE_CONFIG_TOKEN_<N>`.

License
- BSD 3-Clause (see LICENSE file).