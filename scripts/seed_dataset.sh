#!/usr/bin/env bash
set -e

EPISODES=${1:-"50"}

echo "=== Seeding Aegis Training Dataset ==="
echo "Generating ${EPISODES} chaos episodes..."

for i in $(seq 1 "${EPISODES}"); do
    NODE="node-$((RANDOM % 4 + 1))" # node-a..d
    FAULT="latency"
    echo "Episode ${i}/${EPISODES}: Injecting ${FAULT} on ${NODE}..."
    bash "$(dirname "$0")/run_episode.sh" "${FAULT}" "${NODE}" "5" >/dev/null 2>&1 || true
    sleep 3
done

echo "Dataset seeding run complete."
