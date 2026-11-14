# Container Deployment Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan in the existing worktree. The user explicitly requested implementation of the deployment approach agreed in the conversation.

**Goal:** Replace the old Actions workflow with tested Docker-archive-to-VPS deployment on push to main, preserving the server's `.env` and persistent data.

**Architecture:** Actions verifies the project, builds a commit-tagged Linux amd64 image, saves it as a gzip archive, and uploads the archive, checksum and deployment files over verified SSH. A Bash script validates the checksum, loads the image, and backs up the current service before installing the new Compose configuration and image override; `.env` is never written. Nginx remains in the Axentris project and routes wiki.axentris.ru through an external Docker network.

**Tech Stack:** GitHub Actions, OpenSSH, Bash, gzip/SHA-256, Docker Compose v2.20+, existing Go application and SQLite. No image registry.

**Spec:** The user's deployment requirements in this conversation: Docker, no server checkout, push to main, preserve `.env`, provide exact Nginx changes for wiki.axentris.ru.

## Global Constraints

- Production directory: `/opt/knowledgebase`; no source checkout on the VPS.
- `.env`, `data`, and `backups` are never replaced or deleted by deployment.
- Store the selected image in `compose.override.yaml`, independently of `.env`.
- First installation loads the image and prepares files but waits for manual administrator creation before starting HTTP.
- A `.bootstrap-required` marker persists across interrupted/failed bootstrap attempts, even when SQLite has already been initialized.
- Updates require a successful backup before replacing the current configuration; failures must fail Actions.
- Do not automatically downgrade an image after possible SQLite migrations.
- Deploy only main pushes; PRs run verification only. Verify SSH host keys.

## Review Focus

- Existing `.env` with comments and secrets remains byte-for-byte unchanged.
- Archive corruption, image loading, backup, or readiness failure does not report deployment success.
- Back up the old image/configuration, including when the app is stopped.
- A fresh database is not served before administrator creation.
- Parallel deployments cannot overwrite each other's configuration.

## Task 1: Deployment script and workflow

**Files:** `tests/deploy/test_deploy.py`, `deploy/deploy.sh`, `.github/workflows/deploy.yml`; remove `.github/workflows/ci.yml`.

- [x] Write behavior tests for initial install, repeat deployment, missing env/archive, corrupted archive, failed load/backup/readiness, stopped service, invalid image, and concurrent deployment.
- [x] Run tests and confirm failure because the script does not exist.
- [x] Implement checksum/configuration validation, image load, old-service backup, image override installation and readiness checks.
- [x] Replace the old workflow with verification, image archive creation and SSH deployment. Run deployment tests in verification.
- [x] Run deployment tests and validate Actions/Bash/Compose syntax.

## Task 2: Server and Nginx instructions

**Files:** `docs/deployment.md`, `README.md`, `docs/operations.md`, `deploy/.env.example`, `deploy/nginx-compose.fragment.yaml`, `deploy/nginx-wiki.conf.example`.

- [x] Document repository secrets, initial VPS preparation, archive delivery without registry login, administrator bootstrap, backup and failure recovery.
- [x] Provide wiki HTTP/HTTPS server blocks, DNS and certificate bootstrap sequence, and Axentris network changes.
- [x] Point existing production documentation to the new deployment instructions.
- [x] Verify rendered Compose configuration and Nginx syntax with controlled fixtures; review the complete diff.

## Verification and review

- 13 real-script behavior tests pass on Windows/Git Bash and in a Linux Docker container, including archive corruption and load failures.
- actionlint 1.7.11 and ShellCheck pass; production Compose config renders successfully.
- A fixture with an old KB_IMAGE in .env resolves to the new commit image through compose.override.yaml.
- Nginx validates the complete wiki include with a temporary TLS certificate.
- Independent read-only review found an interrupted-bootstrap issue; reproduced it with a failing test, then added the persistent .bootstrap-required gate and verified the full deployment suite again.
- User explicitly selected archive delivery instead of GHCR. Image creation and transfer are in the same job, avoiding any registry or Actions image artifact dependency. Compose pull_policy is never.
- Final actionlint/ShellCheck and real Compose validation pass. Real docker image save → gzip → docker image load roundtrip also passed with an existing local application image. Independent review found no further Important/Critical issues.
- No VPS changes were performed; GitHub/VPS credentials must be configured by the operator.
