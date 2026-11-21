#!/usr/bin/env bash
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
export PROJECT="${PROJECT:-connector-engine}"
if [ -z "${PROFILE:-}" ]; then
  if [ -d "$root/selfhosted" ]; then PROFILE=person; else PROFILE=headless; fi
fi
export PROFILE
if [ "$PROFILE" = headless ]; then COMPOSE_FILE=deploy/compose.yaml; else COMPOSE_FILE=selfhosted/deploy/compose.yaml; fi
env_file="$root/$(dirname "$COMPOSE_FILE")/.env"
if [ -f "$env_file" ]; then
  while IFS='=' read -r key value; do
    case "$key" in '' | \#*) ;; *) [ -n "${!key+x}" ] || export "$key=$value" ;; esac
  done <"$env_file"
fi

export COMPOSE="${COMPOSE:-docker compose -f $COMPOSE_FILE -p $PROJECT}"
export IMAGE="${IMAGE:-connector-engine:local}"
export HEADLESS_IMAGE="${HEADLESS_IMAGE:-connector-engine-headless:local}"
export CN_IMAGE="$IMAGE"
export CN_HEADLESS_IMAGE="$HEADLESS_IMAGE"
if [ "$PROFILE" = headless ]; then
  export RUN_IMAGE="$HEADLESS_IMAGE" ENGINE=engine-headless
else
  export RUN_IMAGE="$IMAGE" ENGINE=engine
fi
export CONSOLE_PORT="${CONSOLE_PORT:-7465}"
export SELFHOSTED_COMPOSE="docker compose -f selfhosted/deploy/compose.yaml -p $PROJECT --profile person"
