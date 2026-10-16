#!/usr/bin/env bash
# disco lane (prototype, #33): bellwether on a fresh Disco server, timed.
#
# Creates a DigitalOcean droplet from the Disco 1-click image, renders bellwether's
# reviewed app profile with `neckbeard disco`, deploys it, probes HTTPS + database +
# worker with bellwether/probe.py, redeploys and probes again (the note must
# survive), then destroys the droplet. Prints a timing table.
#
#   DIGITALOCEAN_TOKEN=... scripts/disco-lane.sh
#
# Needs: curl, jq, ssh, ssh-keygen, python3, go (to build neckbeard). Installs the
# disco CLI into a temp dir if it isn't on PATH. Writes nothing outside $WORK.
# The droplet's ssh host key is generated here and given to it by cloud-init, so ssh
# is pinned (StrictHostKeyChecking=yes), never trust-on-first-use.
set -euo pipefail

: "${DIGITALOCEAN_TOKEN:?set DIGITALOCEAN_TOKEN}"
REGION="${DISCO_LANE_REGION:-}" # default: the first region offering both the size and the image (capacity moves)
SIZE="${DISCO_LANE_SIZE:-s-1vcpu-2gb}"
IMAGE="${DISCO_LANE_IMAGE:-disco}" # the Disco 1-click on the DigitalOcean marketplace
PROJECT=bellwether
REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d -t disco-lane.XXXXXX)"
NAME="neckbeard-disco-lane-$(date -u +%Y%m%d%H%M%S)"
API=https://api.digitalocean.com/v2
export DISCO_CONFIG_PATH="$WORK/disco-config.json" DISCO_NO_UPDATE_CHECK=1

T0=$(date +%s)
declare -a TIMES=()
mark() { TIMES+=("$1|$(( $(date +%s) - T0 ))"); echo "[$(( $(date +%s) - T0 ))s] $1"; }
do_api() { curl -sS --fail-with-body -H "Authorization: Bearer $DIGITALOCEAN_TOKEN" -H "Content-Type: application/json" "$@"; }

DROPLET_ID="" KEY_ID=""
cleanup() {
  [ -n "$DROPLET_ID" ] && [ "$DROPLET_ID" != null ] && do_api -X DELETE "$API/droplets/$DROPLET_ID" >/dev/null && echo "deleted droplet $DROPLET_ID"
  [ -n "$KEY_ID" ] && [ "$KEY_ID" != null ] && do_api -X DELETE "$API/account/keys/$KEY_ID" >/dev/null && echo "deleted ssh key $KEY_ID"
  return 0
}
trap cleanup EXIT

# --- tools ---
(cd "$REPO" && go build -o "$WORK/neckbeard" ./cmd/neckbeard)
if ! command -v disco >/dev/null; then
  curl -fsSL https://cli-assets.letsdisco.dev/install.sh | HOME="$WORK" sh >/dev/null
  PATH="$WORK/.disco/bin:$PATH"
fi

# --- region: where both the size and the image are available right now ---
if [ -z "$REGION" ]; then
  SIZE_REGIONS=$(do_api "$API/sizes?per_page=200" | jq -c --arg s "$SIZE" '[.sizes[] | select(.slug==$s) | .regions[]]')
  REGION=$(do_api "$API/images/$IMAGE" | jq -r --argjson r "$SIZE_REGIONS" '[.image.regions[] | select(. as $x | $r | index($x))] | sort | first // empty')
  [ -n "$REGION" ] || { echo "no region offers both $SIZE and $IMAGE right now"; exit 1; }
fi

# --- droplet with a pinned host key ---
ssh-keygen -q -t ed25519 -N "" -C "$NAME" -f "$WORK/id_ed25519"
ssh-keygen -q -t ed25519 -N "" -C host -f "$WORK/host_ed25519"
USER_DATA="$(printf '#cloud-config\nssh_deletekeys: true\nssh_genkeytypes: []\nssh_keys:\n  ed25519_private: |\n%s\n  ed25519_public: %s\n' \
  "$(sed 's/^/    /' "$WORK/host_ed25519")" "$(cat "$WORK/host_ed25519.pub")")"
KEY_ID=$(do_api -X POST "$API/account/keys" -d "$(jq -n --arg n "$NAME" --arg k "$(cat "$WORK/id_ed25519.pub")" '{name:$n, public_key:$k}')" | jq -r .ssh_key.id)
CREATE=$(do_api -X POST "$API/droplets" -d "$(jq -n --arg n "$NAME" --arg r "$REGION" --arg s "$SIZE" --arg i "$IMAGE" --arg u "$USER_DATA" --argjson k "$KEY_ID" \
  '{name:$n, region:$r, size:$s, image:$i, ssh_keys:[$k], tags:["neckbeard-disco-lane"], user_data:$u}')") || { echo "droplet creation refused: $CREATE"; exit 1; }
DROPLET_ID=$(echo "$CREATE" | jq -r .droplet.id)
[ "$DROPLET_ID" != null ] || { echo "droplet creation failed: $CREATE"; exit 1; }
mark "droplet requested ($DROPLET_ID, $IMAGE, $SIZE, $REGION)"

IP=""
until [ -n "$IP" ]; do
  sleep 5
  IP=$(do_api "$API/droplets/$DROPLET_ID" | jq -r '.droplet | select(.status=="active") | .networks.v4[] | select(.type=="public") | .ip_address' | head -1)
done
echo "$IP $(cut -d' ' -f1,2 "$WORK/host_ed25519.pub")" > "$WORK/known_hosts"
SSH=(ssh -i "$WORK/id_ed25519" -o UserKnownHostsFile="$WORK/known_hosts" -o StrictHostKeyChecking=yes -o HostKeyAlgorithms=ssh-ed25519 -o BatchMode=yes -o ConnectTimeout=10 "root@$IP")
mark "droplet active ($IP)"
until "${SSH[@]}" true 2>/dev/null; do sleep 5; done
mark "ssh up"

# --- wait for the image's first boot (provisions a domain, inits disco) ---
until "${SSH[@]}" 'test -s /etc/disco-image/disco-host && grep -q "Created API key" /etc/disco-image/init-output.log' 2>/dev/null; do sleep 5; done
HOST=$("${SSH[@]}" cat /etc/disco-image/disco-host)
until curl -fsS -m 10 "https://$HOST/" >/dev/null 2>&1; do sleep 5; done
mark "disco ready (https://$HOST, $("${SSH[@]}" cat /etc/disco-image/daemon-version))"

# a cli invite, minted on the box with the key first boot created
INVITE=$("${SSH[@]}" 'K=$(grep -o "Created API key: [a-z0-9]\{32\}" /etc/disco-image/init-output.log | head -1 | awk "{print \$4}"); \
  curl -fsS -X POST "https://$(cat /etc/disco-image/disco-host)/api/api-key-invites" -H "Authorization: Basic $(echo -n "$K:" | base64)" \
  -H "Content-Type: application/json" -d "{\"name\":\"neckbeard-disco-lane\"}" | jq -r .apiKeyInvite.url')
disco invite:accept "$INVITE" >/dev/null
mark "cli connected"

# the 1-click may lag behind the latest daemon (0.32.0 in 2026-10, no folder upload): upgrade first
FROM=$(disco meta:info --json | jq -r .version)
[ -n "$FROM" ] && [ "$FROM" != null ] || { echo "could not read the daemon version"; exit 1; }
disco meta:upgrade >/dev/null
DEADLINE=$(( $(date +%s) + 600 ))
until V=$(disco meta:info --json 2>/dev/null | jq -r .version) && [ -n "$V" ] && [ "$V" != null ] && [ "$V" != "$FROM" ]; do
  [ "$(date +%s)" -lt "$DEADLINE" ] || { echo "daemon still $FROM after 10 minutes"; exit 1; }
  sleep 5
done
mark "upgraded disco $FROM -> $V"

# --- render + deploy ---
SRC="$WORK/src"
git -C "$REPO" ls-files -z | (cd "$REPO" && xargs -0 tar cf -) | (mkdir -p "$SRC" && tar xf - -C "$SRC")
(cd "$SRC" && "$WORK/neckbeard" disco -profile bellwether/app-profile.yaml -project "$PROJECT" -force)
# the cron runs at 03:00 UTC: for the lane only, every minute, so the schedule can be seen firing
CRON=$(jq -r '.services | to_entries[] | select(.value.type=="cron") | .key' "$SRC/disco.json" | head -1)
jq --arg c "$CRON" '.services[$c].schedule = "* * * * *"' "$SRC/disco.json" > "$SRC/disco.json.lane" && mv "$SRC/disco.json.lane" "$SRC/disco.json"
disco projects:add --name "$PROJECT" >/dev/null
disco postgres:create --project "$PROJECT" >/dev/null
disco env:set --project "$PROJECT" APP_ENV=dev >/dev/null # never deployed yet: no redeploy
mark "project + postgres created"
(cd "$SRC" && disco deploy --project "$PROJECT" --dir . >"$WORK/deploy-1.log" 2>&1) || true
grep -q "Deployment complete" "$WORK/deploy-1.log" || { tail -30 "$WORK/deploy-1.log"; exit 1; }
mark "first deploy complete"

DOMAIN=$(disco domains:list --project "$PROJECT" | head -1)
printf '{"health_url":"https://%s/healthz","environment":"dev","expected_version":"0.1.0"}\n' "$DOMAIN" > "$WORK/release.json"
until curl -fsS -m 10 "https://$DOMAIN/healthz" >/dev/null 2>&1; do sleep 3; done
python3 "$REPO/bellwether/probe.py" --release "$WORK/release.json" --note "$WORK/note.json" --create
mark "probe 1 passed (https, db, worker)"

# the cron: wait for a tick and look for bellwether's report line in the project logs
# (vector streams the cron container's output while it runs)
CRON_OK=""
for i in 1 2 3; do
  timeout 75 disco logs --project "$PROJECT" >"$WORK/logs-$i.txt" 2>&1 || true
  if grep -m1 "bellwether report (env dev)" "$WORK/logs-$i.txt" >"$WORK/cron-line.txt"; then
    CRON_OK=1; break
  fi
done
[ -n "$CRON_OK" ] || { echo "cron: no report line in 3 minutes of logs"; tail -20 "$WORK/logs-3.txt"; exit 1; }
mark "cron $CRON fired: $(grep -o 'bellwether report.*' "$WORK/cron-line.txt")"

(cd "$SRC" && disco deploy --project "$PROJECT" --dir . >"$WORK/deploy-2.log" 2>&1) || true
grep -q "Deployment complete" "$WORK/deploy-2.log" || { tail -30 "$WORK/deploy-2.log"; exit 1; }
mark "redeploy complete"
python3 "$REPO/bellwether/probe.py" --release "$WORK/release.json" --note "$WORK/note.json"
mark "probe 2 passed (the note survived the release)"

echo
echo "| step | seconds since start |"
echo "|---|---|"
for t in "${TIMES[@]}"; do echo "| ${t%%|*} | ${t##*|} |"; done
echo
echo "cron ($CRON): rendered with its real schedule, run every minute in the lane; its report line (counts from the db) is above"
echo "artifacts: $WORK"
