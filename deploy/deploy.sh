#!/usr/bin/env bash
# Executed from the uploaded staging directory. Never writes the server's .env.
set -euo pipefail
umask 077

die() { printf '%s\n' "$*" >&2; exit 1; }

[[ $# == 2 ]] || die "Usage: bash deploy.sh /opt/knowledgebase knowledgebase:commit"
deploy_dir=$1
image=$2
[[ "$image" =~ ^knowledgebase:[a-f0-9]{40}$ ]] || die "Expected a local knowledgebase image tagged with a full commit SHA"
stage_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd -- "$deploy_dir"
[[ -f .env ]] || die "Create $deploy_dir/.env manually before deployment; it will never be overwritten"
[[ -d data && -d backups ]] || die "Prepare data and backups directories (owner 65532:65532) first"
mkdir -- .deploy-lock 2>/dev/null || die "Another deployment holds $deploy_dir/.deploy-lock"
cleanup() {
    rm -f -- .compose.next.yaml .compose.next.override.yaml
    rmdir -- .deploy-lock
}
trap cleanup EXIT

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
    [[ -f compose.yaml ]] || die "Existing database without compose.yaml: back it up and configure the existing installation first"
    # Explicit files prevent unrelated COMPOSE_FILE settings from selecting another project.
    current=(docker compose --env-file .env -f compose.yaml)
    if [[ -f compose.override.yaml ]]; then
        current+=(-f compose.override.yaml)
    fi
    running=$("${current[@]}" ps --status running -q knowledgebase-app)
    if [[ -n "$running" ]]; then
        "${current[@]}" exec -T knowledgebase-app /app/kb backup
    else
        "${current[@]}" run --rm --no-deps -T knowledgebase-app backup
    fi
fi

# Preserve previous manifests for diagnosis/manual recovery, never overwrite .env.
if [[ -f compose.yaml ]]; then
    cp -p -- compose.yaml compose.previous.yaml
fi
if [[ -f compose.override.yaml ]]; then
    cp -p -- compose.override.yaml compose.previous.override.yaml
fi
mv -- .compose.next.yaml compose.yaml
mv -- .compose.next.override.yaml compose.override.yaml
cp -- "$stage_dir/.env.example" .env.example

if [[ ! -f data/knowledgebase.db || -f .bootstrap-required ]]; then
    touch -- .bootstrap-required
    printf '%s\n' "Image and configuration prepared. Create the first administrator before starting HTTP; see docs/deployment.md."
    exit 0
fi

docker compose --env-file .env -f compose.yaml -f compose.override.yaml \
    up -d --no-deps --wait --wait-timeout 120 knowledgebase-app
docker compose --env-file .env -f compose.yaml -f compose.override.yaml \
    exec -T knowledgebase-app /app/kb check
printf 'Deployed %s\n' "$image"
