#!/usr/bin/env bash
# Runs one benchmark cell: a task prompt against a fixture app, with or without
# the neckbeard plugin, in a fresh copy of the app, then scores the output.
#
#   benchmarks/run.sh prepare                       # fetch/copy the fixture apps
#   benchmarks/run.sh run <task> <app> <arm> <n>    # one run; arm = plugin|bare
#   benchmarks/run.sh cell <task> <app> [runs]       # both arms, N runs each (default 3), serially
#   benchmarks/run.sh matrix [runs] [parallel]       # every cell, N runs each, PARALLEL agents at once (default 3, 4)
#   benchmarks/run.sh score <run-dir>                # re-score one run without re-running the agent
#
# Both arms get the same prompt, the same model, the same tools and the same
# scrubbed environment (no cloud credentials, cloud CLIs denied). The plugin arm
# additionally loads this repository as a Claude Code plugin and has the
# neckbeard CLI on PATH. Nothing here deploys anything.
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
repo="$(cd "$here/.." && pwd)"
work="${BENCH_WORK:-$here/.work}"
runs_dir="$work/runs"
apps_dir="$work/apps"
CLAUDE_BIN="${CLAUDE_BIN:-$(command -v claude || true)}"
NECKBEARD_BIN="${NECKBEARD_BIN:-$work/bin/neckbeard}"
MAX_TURNS="${MAX_TURNS:-120}"
RUN_TIMEOUT="${RUN_TIMEOUT:-45m}"
# One provider cache for every tofu init in every run, both arms and the
# scorer: without it each of the six roots in each run downloads the AWS
# provider again, which was most of the idle time in the first matrix.
export TF_PLUGIN_CACHE_DIR="${TF_PLUGIN_CACHE_DIR:-$work/tofu-plugin-cache}"
UMAMI_SHA="${UMAMI_SHA:-559e3c607c23d2c72d9bd4f6481bda428255323c}"
# A clean PATH for both arms: system and Homebrew tools only, so an installed
# neckbeard on the operator's machine cannot leak into the bare arm.
CLEAN_PATH="/opt/homebrew/bin:/opt/homebrew/sbin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

die() { echo "run.sh: $*" >&2; exit 1; }

prepare() {
  mkdir -p "$apps_dir" "$work/bin"
  # bellwether: the repo's own reference app, as a standalone module without
  # the profile and config the plugin would otherwise find ready-made.
  rm -rf "$apps_dir/bellwether" && mkdir -p "$apps_dir/bellwether"
  cp "$repo/bellwether/main.go" "$repo/bellwether/Dockerfile" "$apps_dir/bellwether/"
  sed -i.bak 's#COPY bellwether/#COPY #; s#./bellwether#.#g' "$apps_dir/bellwether/Dockerfile" && rm -f "$apps_dir/bellwether/Dockerfile.bak"
  (cd "$apps_dir/bellwether" && go mod init bellwether >/dev/null 2>&1 && GOFLAGS=-mod=mod go mod tidy >/dev/null 2>&1 && go build ./... && rm -f bellwether)
  # python: the small onboarding fixture.
  rm -rf "$apps_dir/python" && cp -R "$repo/cmd/neckbeard/testdata/onboarding/python" "$apps_dir/python"
  # umami: a real application nobody here wrote, pinned to one commit.
  if [ ! -d "$apps_dir/umami/.git" ]; then
    git clone -q --depth 1 https://github.com/umami-software/umami "$apps_dir/umami"
  fi
  (cd "$apps_dir/umami" && git fetch -q --depth 1 origin "$UMAMI_SHA" && git checkout -q "$UMAMI_SHA")
  # the plugin arm's CLI, built from this checkout.
  (cd "$repo" && go build -o "$work/bin/neckbeard" ./cmd/neckbeard)
  echo "prepared: $(ls "$apps_dir" | tr '\n' ' ')| neckbeard $("$work/bin/neckbeard" version)"
}

run_one() {
  local task="$1" app="$2" arm="$3" n="$4"
  [ -n "$CLAUDE_BIN" ] || die "claude not found; set CLAUDE_BIN"
  local prompt="$here/prompts/$task.md"
  [ -f "$prompt" ] || die "no prompt for task $task"
  local dir="$runs_dir/$task/$app/$arm/$n"
  rm -rf "$dir" && mkdir -p "$dir/app"
  if [ "$app" = "none" ]; then
    mkdir -p "$dir/app/site" && printf '<!doctype html><title>example</title><h1>hello</h1>\n' > "$dir/app/site/index.html"
  else
    [ -d "$apps_dir/$app" ] || die "app $app not prepared"
    (cd "$apps_dir/$app" && tar --exclude=.git --exclude=node_modules -cf - .) | (cd "$dir/app" && tar -xf -)
  fi
  (cd "$dir/app" && git init -q && git add -A >/dev/null && git -c user.name=bench -c user.email=bench@example.invalid commit -q -m "fixture" >/dev/null)

  local path="$CLEAN_PATH" plugin_args=()
  if [ "$arm" = "plugin" ]; then
    [ -x "$NECKBEARD_BIN" ] || die "neckbeard CLI missing at $NECKBEARD_BIN (run prepare)"
    path="$(dirname "$NECKBEARD_BIN"):$CLEAN_PATH"
    plugin_args=(--plugin-dir "$repo")
  elif [ "$arm" != "bare" ]; then
    die "arm must be plugin or bare"
  fi

  mkdir -p "$TF_PLUGIN_CACHE_DIR"
  local start end
  start="$(date +%s)"
  # No cloud credentials reach the agent, and the cloud CLIs are denied on top
  # of that. Nothing in these runs should need them, and nothing may use them.
  set +e
  (cd "$dir/app" && env -u AWS_PROFILE -u AWS_ACCESS_KEY_ID -u AWS_SECRET_ACCESS_KEY -u AWS_SESSION_TOKEN -u GOOGLE_APPLICATION_CREDENTIALS \
      PATH="$path" AWS_CONFIG_FILE=/dev/null AWS_SHARED_CREDENTIALS_FILE=/dev/null AWS_EC2_METADATA_DISABLED=true \
      AZURE_CONFIG_DIR="$dir/.no-azure" CLOUDSDK_CONFIG="$dir/.no-gcloud" NECKBEARD_BENCH_ARM="$arm" TF_PLUGIN_CACHE_DIR="$TF_PLUGIN_CACHE_DIR" \
      timeout "$RUN_TIMEOUT" "$CLAUDE_BIN" -p "$(cat "$prompt")" \
        --output-format stream-json --verbose --max-turns "$MAX_TURNS" --no-session-persistence \
        --permission-mode bypassPermissions --permission-prompts none \
        --setting-sources user \
        --disallowedTools "Bash(aws *)" "Bash(az *)" "Bash(gcloud *)" "Bash(gh *)" "Bash(git push*)" "Bash(terraform apply*)" "Bash(tofu apply*)" \
        ${MODEL:+--model "$MODEL"} "${plugin_args[@]}" \
        > "$dir/events.jsonl" 2> "$dir/stderr.log")
  local status=$?
  set -e
  end="$(date +%s)"
  printf '{"task":"%s","app":"%s","arm":"%s","n":%s,"exit":%s,"wall_seconds":%s,"started":"%s"}\n' \
    "$task" "$app" "$arm" "$n" "$status" "$((end-start))" "$(date -u -r "$start" +%Y-%m-%dT%H:%M:%SZ)" > "$dir/run.json"
  (cd "$dir/app" && git add -A >/dev/null && git diff --cached --stat > "$dir/changes.txt" || true)
  if [ "${SCORE_LATER:-0}" = 1 ]; then
    echo "$task/$app/$arm/$n exit=$status wall=$((end-start))s (scoring deferred)"
  else
    score_one "$dir"
  fi
}

score_one() {
  local dir="$1"
  PATH="$CLEAN_PATH" python3 "$here/score.py" "$dir" > "$dir/metrics.json.new" && mv "$dir/metrics.json.new" "$dir/metrics.json"
  echo "$(echo "$dir" | sed "s#$runs_dir/##") $(python3 -c 'import json,sys; m=json.load(open(sys.argv[1])); print("exit=%s wall=%ss cost=$%.2f turns=%s v0=%s" % (m["run"]["exit"], m["run"]["wall_seconds"], m["agent"]["cost_usd"] or 0, m["agent"]["turns"], m["v0"]["passes"]))' "$dir/metrics.json")"
}

# The matrix is a queue, not a loop: every run is a job, PARALLEL agents work
# the queue at once with the slow bare runs interleaved among the fast plugin
# ones, and scoring happens afterwards in its own parallel pass so no agent
# ever waits on checkov. Concurrency is bounded by your Anthropic rate limit;
# four was comfortable on one account, six showed rate-limit events.
matrix() {
  local runs="${1:-3}" parallel="${2:-4}"
  local jobs=()
  for n in $(seq 1 "$runs"); do
    for cell in "onboard bellwether" "onboard umami" "onboard python" "needs bellwether" "needs umami" "static-site none"; do
      for arm in bare plugin; do
        jobs+=("$cell $arm $n")
      done
    done
  done
  printf '%s\n' "${jobs[@]}" | SCORE_LATER=1 xargs -P "$parallel" -L 1 bash -c 'bash "$0" run "$@" || echo "FAILED $*" >&2' "$here/run.sh"
  find "$runs_dir" -mindepth 5 -maxdepth 5 -name run.json | xargs -P "$parallel" -I{} bash -c 'bash "$0" score "$(dirname "$1")"' "$here/run.sh" {}
}

cell() {
  local task="$1" app="$2" runs="${3:-3}"
  for n in $(seq 1 "$runs"); do
    for arm in plugin bare; do
      run_one "$task" "$app" "$arm" "$n" || echo "FAILED $task/$app/$arm/$n" >&2
    done
  done
}

case "${1:-}" in
  prepare) prepare ;;
  run) shift; [ $# -eq 4 ] || die "run <task> <app> <arm> <n>"; run_one "$@" ;;
  cell) shift; [ $# -ge 2 ] || die "cell <task> <app> [runs]"; cell "$@" ;;
  score) shift; [ $# -eq 1 ] || die "score <run-dir>"; score_one "$1" ;;
  matrix) shift; matrix "$@" ;;
  *) sed -n "2,14p" "$0" | sed "s/^# \{0,1\}//"; exit 2 ;;
esac
