#!/bin/sh
# Regenerates bot/deploy/configmap.yaml from bot/scenarios/*.yaml.
# kubectl-free equivalent of:
#   kubectl create configmap bot-scenarios --from-file=bot/scenarios/ --dry-run=client -o yaml
# Run from the repository root: sh bot/deploy/gen-configmap.sh
set -eu
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="$ROOT/bot/deploy/configmap.yaml"
{
  echo "# GENERATED from bot/scenarios/*.yaml by bot/deploy/gen-configmap.sh. Do not edit by hand."
  echo "# Mounted at /scenarios in the bot pod; select a file with BOT_SCENARIO_FILE."
  echo "apiVersion: v1"
  echo "kind: ConfigMap"
  echo "metadata:"
  echo "  name: bot-scenarios"
  echo "  labels:"
  echo "    app.kubernetes.io/name: leaderboard-bot"
  echo "data:"
  for f in "$ROOT"/bot/scenarios/*.yaml; do
    echo "  $(basename "$f"): |"
    sed 's/^/    /' "$f"
  done
} > "$OUT"
echo "wrote $OUT"
