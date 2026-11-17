#!/usr/bin/env bash
# Executed from the uploaded staging directory. Never writes the server's .env.
set -euo pipefail
umask 077

die() { printf '%s\n' "$*" >&2; exit 1; }

[[ $# == 2 || $# == 3 ]] || die "Usage: bash deploy.sh /var/www/knowledgebase knowledgebase:commit [/var/www/axentris_core]"
deploy_dir=$1
image=$2
core_dir=${3:-/var/www/axentris_core}
[[ "$image" =~ ^knowledgebase:[a-f0-9]{40}$ ]] || die "Expected a local knowledgebase image tagged with a full commit SHA"
[[ -f "$core_dir/docker-compose.yml" && -s "$core_dir/.env" ]] || die "Deploy axentris_core and prepare its .env first"
core_dir=$(cd -- "$core_dir" && pwd)
stage_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd -- "$deploy_dir"
deploy_dir=$PWD
[[ -f .env ]] || die "Create $deploy_dir/.env manually before deployment; it will never be overwritten"
[[ -d data && -d backups ]] || die "Prepare data and backups directories (owner 65532:deploy, mode 2750) first"
mkdir -- .deploy-lock 2>/dev/null || die "Another deployment holds $deploy_dir/.deploy-lock"
cleanup() {
    rm -f -- "$deploy_dir/.compose.next.yaml" "$deploy_dir/.compose.next.override.yaml"
    rmdir -- "$deploy_dir/.deploy-lock"
}
trap cleanup EXIT
# Core and wiki deployments must not recreate containers concurrently.
exec 9> "$core_dir/.deploy.lock"
flock -w 600 9 || die "Timed out waiting for the core deployment lock"

# Refuse incomplete/corrupted uploads before loading or replacing anything.
[[ -f "$stage_dir/knowledgebase-image.tar.gz" ]] || die "Missing Docker image archive"
[[ -f "$stage_dir/knowledgebase-image.tar.gz.sha256" ]] || die "Missing archive checksum"
(cd -- "$stage_dir" && sha256sum -c knowledgebase-image.tar.gz.sha256)

# Validate staged configuration without changing the running deployment.
cp -- "$stage_dir/compose.yaml" .compose.next.yaml
printf 'services:\n  knowledgebase-app:\n    image: %s\n    pull_policy: never\n' "$image" > .compose.next.override.yaml
docker compose --env-file .env -f .compose.next.yaml -f .compose.next.override.yaml config --quiet
docker image load --input "$stage_dir/knowledgebase-image.tar.gz"
docker image inspect "$image" >/dev/null

if [[ -f data/knowledgebase.db ]]; then
    [[ -f docker-compose.yml ]] || die "Existing database without docker-compose.yml: back it up and configure the existing installation first"
    # Explicit files prevent unrelated COMPOSE_FILE settings from selecting another project.
    current=(docker compose --project-name axentris_core --profile knowledgebase --env-file .env -f docker-compose.yml)
    if [[ -f docker-compose.override.yml ]]; then
        current+=(-f docker-compose.override.yml)
    fi
    running=$("${current[@]}" ps --status running -q knowledgebase-app)
    if [[ -n "$running" ]]; then
        "${current[@]}" exec -T knowledgebase-app /app/kb backup
    else
        "${current[@]}" run --rm --no-deps -T knowledgebase-app backup
    fi
fi

# Preserve previous manifests for diagnosis/manual recovery, never overwrite .env.
if [[ -f docker-compose.yml ]]; then
    cp -p -- docker-compose.yml docker-compose.previous.yml
fi
if [[ -f docker-compose.override.yml ]]; then
    cp -p -- docker-compose.override.yml docker-compose.previous.override.yml
fi
mv -- .compose.next.yaml docker-compose.yml
mv -- .compose.next.override.yaml docker-compose.override.yml
cp -- "$stage_dir/.env.example" .env.example

bootstrap=false
if [[ ! -f data/knowledgebase.db || -f .bootstrap-required ]]; then
    touch -- .bootstrap-required
    bootstrap=true
fi

# All long-running containers belong to the single core Compose project.
export KNOWLEDGEBASE_PROJECT_DIR="$deploy_dir"
cd -- "$core_dir"
core=(docker compose --env-file .env -f docker-compose.yml)
"${core[@]}" config --quiet
"${core[@]}" up -d --build --no-deps --wait --wait-timeout 120 nginx
"${core[@]}" exec -T nginx sh /opt/certificates/nginx-certificates.sh reload
"${core[@]}" up -d --no-deps certbot
"${core[@]}" restart certbot

if [[ "$bootstrap" == true ]]; then
    printf '%s\n' "Image and configuration prepared. Create the first administrator before starting HTTP; see docs/deployment.md."
    exit 0
fi

"${core[@]}" up -d --no-deps --wait --wait-timeout 120 knowledgebase-app
"${core[@]}" exec -T knowledgebase-app /app/kb check
printf 'Deployed %s\n' "$image"
