#!/usr/bin/env bash
# disco lane (prototype, #33): bellwether on a fresh Disco server, timed.
#
# Creates a DigitalOcean droplet from the Disco 1-click image, renders bellwether's
# reviewed app profile with `neckbeard disco`, deploys it, probes HTTPS + database +
# worker with bellwether/probe.py, checks the cron fires, deploys release 2 (a new
# version), probes that the release 1 note survived and that release 2's worker
# processes a fresh one, then destroys the droplet and checks nothing is left.
# Every wait has a deadline; a failed teardown fails the run. Prints a timing table.
#
#   DIGITALOCEAN_TOKEN=... scripts/disco-lane.sh
#
# Needs: curl, jq, ssh, ssh-keygen, python3, go (to build neckbeard). Installs the
# disco CLI into a temp dir if it isn't on PATH. Writes nothing outside $WORK.
# The droplet's ssh host key is generated here and given to it by cloud-init, so ssh
# is pinned (StrictHostKeyChecking=yes), never trust-on-first-use.
set -euo pipefail
umask 077

: "${DIGITALOCEAN_TOKEN:?set DIGITALOCEAN_TOKEN}"
REGION="${DISCO_LANE_REGION:-}" # default: the first region offering both the size and the image (capacity moves)
SIZE="${DISCO_LANE_SIZE:-s-1vcpu-2gb}"
IMAGE="${DISCO_LANE_IMAGE:-disco}" # the Disco 1-click on the DigitalOcean marketplace
DAEMON="${DISCO_LANE_DAEMON:-0.34.0}" # daemon version the lane runs on (first with deploy --dir)
PROJECT=bellwether
REPO="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$(mktemp -d -t disco-lane.XXXXXX)"
NAME="neckbeard-disco-lane-$(date -u +%Y%m%d%H%M%S)-$(openssl rand -hex 3)" # also this run's droplet tag
API=https://api.digitalocean.com/v2
printf 'Authorization: Bearer %s\n' "$DIGITALOCEAN_TOKEN" > "$WORK/auth.header" # keeps the token out of argv
export DISCO_CONFIG_PATH="$WORK/disco-config.json" DISCO_NO_UPDATE_CHECK=1

T0=$(date +%s)
declare -a TIMES=()
mark() { TIMES+=("$1|$(( $(date +%s) - T0 ))"); echo "[$(( $(date +%s) - T0 ))s] $1"; }
do_api() { curl -sS -m 30 --fail-with-body -H @"$WORK/auth.header" -H "Content-Type: application/json" "$@"; }
# api_code METHOD URL: the http status only (teardown decides on it)
api_code() { curl -sS -m 30 -o /dev/null -w '%{http_code}' -X "$1" -H @"$WORK/auth.header" "$2" || echo 000; }
# wait_for SECONDS WHAT CMD...: run CMD every 5 s until it succeeds, fail after SECONDS
wait_for() {
  local end=$(( $(date +%s) + $1 )) what=$2; shift 2
  until "$@"; do
    [ "$(date +%s)" -lt "$end" ] || { echo "timed out: $what"; return 1; }
    sleep 5
  done
}

# teardown: by id and by this run's tag (covers a lost create response), retried, then
# verified gone. a teardown that cannot be verified fails the run, whatever happened before.
KEY_ID=""
# only a well-formed answer counts: an error body has no .droplets, which must not read as "none left"
gone_droplets() {
  local n
  n=$(do_api "$API/droplets?tag_name=$NAME" | jq -e '.droplets | if type == "array" then length else error("no droplet list") end') || return 1
  [ "$n" = 0 ]
}
gone_key() { [ "$(api_code GET "$API/account/keys/$KEY_ID")" = 404 ]; }
cleanup() {
  local rc=$? ok=1 i
  for i in 1 2 3; do
    case "$(api_code DELETE "$API/droplets?tag_name=$NAME")" in 204|404) break;; esac
    sleep 5
  done
  wait_for 180 "droplets tagged $NAME still exist" gone_droplets && echo "no droplet left (tag $NAME)" || ok=0
  if [ -z "$KEY_ID" ]; then # the create response may have been lost: find the key by name
    KEY_ID=$(do_api "$API/account/keys?per_page=200" | jq -er --arg n "$NAME" '.ssh_keys | if type == "array" then ([.[] | select(.name==$n) | .id] | first // "") else error("no key list") end') \
      || { echo "could not list ssh keys"; ok=0; KEY_ID=""; }
  fi
  if [ -n "$KEY_ID" ]; then
    for i in 1 2 3; do
      case "$(api_code DELETE "$API/account/keys/$KEY_ID")" in 204|404) break;; esac
      sleep 5
    done
    wait_for 60 "ssh key $KEY_ID still exists" gone_key && echo "no ssh key left ($KEY_ID)" || ok=0
  fi
  [ "$ok" = 1 ] || { echo "TEARDOWN NOT VERIFIED: check droplets tagged $NAME and ssh keys named $NAME"; exit 1; }
  exit "$rc"
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
KEY_ID=$(do_api -X POST "$API/account/keys" -d "$(jq -n --arg n "$NAME" --arg k "$(cat "$WORK/id_ed25519.pub")" '{name:$n, public_key:$k}')" | jq -r '.ssh_key.id // empty')
[ -n "$KEY_ID" ] || { echo "ssh key creation failed"; exit 1; }
CREATE=$(do_api -X POST "$API/droplets" -d "$(jq -n --arg n "$NAME" --arg r "$REGION" --arg s "$SIZE" --arg i "$IMAGE" --arg u "$USER_DATA" --argjson k "$KEY_ID" \
  '{name:$n, region:$r, size:$s, image:$i, ssh_keys:[$k], tags:["neckbeard-disco-lane", $n], user_data:$u}')") || { echo "droplet creation refused: $CREATE"; exit 1; }
DROPLET_ID=$(echo "$CREATE" | jq -r .droplet.id)
[ "$DROPLET_ID" != null ] || { echo "droplet creation failed: $CREATE"; exit 1; }
mark "droplet requested ($DROPLET_ID, $IMAGE, $SIZE, $REGION)"

IP=""
active_ip() { IP=$(do_api "$API/droplets/$DROPLET_ID" | jq -r '.droplet | select(.status=="active") | .networks.v4[] | select(.type=="public") | .ip_address' | head -1); [ -n "$IP" ]; }
wait_for 300 "droplet $DROPLET_ID active with a public ip" active_ip
echo "$IP $(cut -d' ' -f1,2 "$WORK/host_ed25519.pub")" > "$WORK/known_hosts"
SSH=(ssh -i "$WORK/id_ed25519" -o UserKnownHostsFile="$WORK/known_hosts" -o StrictHostKeyChecking=yes -o HostKeyAlgorithms=ssh-ed25519 -o BatchMode=yes -o ConnectTimeout=10 "root@$IP")
mark "droplet active ($IP)"
ssh_up() { "${SSH[@]}" true 2>/dev/null; }
wait_for 300 "ssh on $IP" ssh_up
mark "ssh up"

# --- wait for the image's first boot (provisions a domain, inits disco) ---
# (a stopgap: the first api key is read from the image's first-boot log, as the image's own
#  invite helper does; `disco init` against a plain droplet will replace this)
first_boot() { "${SSH[@]}" 'test -s /etc/disco-image/disco-host && grep -q "Created API key" /etc/disco-image/init-output.log' 2>/dev/null; }
wait_for 600 "the image's first boot (domain + disco init)" first_boot
HOST=$("${SSH[@]}" cat /etc/disco-image/disco-host)
https_up() { curl -fsS -m 10 "https://$1/${2:-}" >/dev/null 2>&1; }
wait_for 300 "https://$HOST" https_up "$HOST"
mark "disco ready (https://$HOST, $("${SSH[@]}" cat /etc/disco-image/daemon-version))"

# a cli invite, minted on the box with the key first boot created
INVITE=$("${SSH[@]}" 'K=$(grep -o "Created API key: [a-z0-9]\{32\}" /etc/disco-image/init-output.log | head -1 | awk "{print \$4}"); \
  curl -fsS -X POST "https://$(cat /etc/disco-image/disco-host)/api/api-key-invites" -H "Authorization: Basic $(echo -n "$K:" | base64)" \
  -H "Content-Type: application/json" -d "{\"name\":\"neckbeard-disco-lane\"}" | jq -r .apiKeyInvite.url')
disco invite:accept "$INVITE" >/dev/null
mark "cli connected"

# the 1-click may lag behind (0.32.0 in 2026-10, no folder upload): move to the pinned daemon
FROM=$(disco meta:info --json | jq -r .version)
[ -n "$FROM" ] && [ "$FROM" != null ] || { echo "could not read the daemon version"; exit 1; }
if [ "$FROM" = "$DAEMON" ]; then
  mark "disco already $DAEMON"
else
  disco meta:upgrade --image "letsdiscodev/daemon:$DAEMON" >/dev/null
  on_daemon() { [ "$(disco meta:info --json 2>/dev/null | jq -r .version)" = "$DAEMON" ]; }
  wait_for 600 "daemon $FROM -> $DAEMON" on_daemon
  mark "upgraded disco $FROM -> $DAEMON"
fi

# --- render + deploy ---
SRC="$WORK/src"
mkdir -p "$SRC"
(cd "$REPO" && git ls-files -z | tar --null -T - -cf -) | tar xf - -C "$SRC" # one archive, every tracked file
(cd "$SRC" && "$WORK/neckbeard" disco -profile bellwether/app-profile.yaml -project "$PROJECT" -force)
# the cron runs at 03:00 UTC: for the lane only, every minute, so the schedule can be seen firing
CRON=$(jq -r '.services | to_entries[] | select(.value.type=="cron") | .key' "$SRC/disco.json" | head -1)
jq --arg c "$CRON" '.services[$c].schedule = "* * * * *"' "$SRC/disco.json" > "$SRC/disco.json.lane" && mv "$SRC/disco.json.lane" "$SRC/disco.json"
disco projects:add --name "$PROJECT" >/dev/null
disco postgres:create --project "$PROJECT" >/dev/null
disco env:set --project "$PROJECT" APP_ENV=dev >/dev/null # never deployed yet: no redeploy
mark "project + postgres created"
# deploy N: the cli must exit 0 and print its completion line
deploy() {
  local rc=0
  (cd "$SRC" && disco deploy --project "$PROJECT" --dir . >"$WORK/deploy-$1.log" 2>&1) || rc=$?
  [ "$rc" = 0 ] && grep -q "Deployment complete" "$WORK/deploy-$1.log" || { echo "deploy $1: exit $rc"; tail -30 "$WORK/deploy-$1.log"; exit 1; }
}
deploy 1
mark "first deploy complete"

DOMAIN=$(disco domains:list --project "$PROJECT" | head -1)
printf '{"health_url":"https://%s/healthz","environment":"dev","expected_version":"0.1.0"}\n' "$DOMAIN" > "$WORK/release.json"
wait_for 300 "https://$DOMAIN/healthz" https_up "$DOMAIN" healthz
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

# release 2 is a new version (the image's APP_VERSION default, in the lane's copy only)
sed 's/^ARG APP_VERSION=0\.1\.0$/ARG APP_VERSION=0.2.0/' "$SRC/bellwether/Dockerfile" > "$SRC/bellwether/Dockerfile.lane"
mv "$SRC/bellwether/Dockerfile.lane" "$SRC/bellwether/Dockerfile"
grep -qx 'ARG APP_VERSION=0.2.0' "$SRC/bellwether/Dockerfile" || { echo "could not bump APP_VERSION in the lane's Dockerfile"; exit 1; }
deploy 2
mark "release 2 (0.2.0) deployed"
jq '.expected_version = "0.2.0"' "$WORK/release.json" > "$WORK/release-2.json"
python3 "$REPO/bellwether/probe.py" --release "$WORK/release-2.json" --note "$WORK/note.json"
python3 "$REPO/bellwether/probe.py" --release "$WORK/release-2.json" --note "$WORK/note-2.json" --create
mark "probe 2 passed (0.2.0 serving, release 1 note intact, release 2 worker processed a new note)"

echo
echo "| step | seconds since start |"
echo "|---|---|"
for t in "${TIMES[@]}"; do echo "| ${t%%|*} | ${t##*|} |"; done
echo
echo "cron ($CRON): rendered with its real schedule, run every minute in the lane; its report line (counts from the db) is above"
echo "artifacts: $WORK"
