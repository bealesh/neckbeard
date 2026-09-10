#!/usr/bin/env python3
"""Test GitLab's approval identity rule using an existing identity-only preflight.

The private test project must already have preflight-dev/stg/prd jobs on main,
a protected prd environment with one required approval, and self-approval disabled.
A temporary project bot starts the pipeline; the signed-in Maintainer approves it.
This proves platform enforcement between accounts, not an independent human review.
The token is kept in memory and revoked on exit. Its ID is saved for cleanup if
this process is forcibly killed. This probe never configures cloud identities.
"""
import argparse, datetime, json, os, pathlib, subprocess, time
parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--project', required=True, help='numeric ID of the disposable private preflight project')
parser.add_argument('--expected-user', required=True, help='signed-in Maintainer username')
parser.add_argument('--evidence', required=True, type=pathlib.Path)
args = parser.parse_args()
if not args.project.isdecimal():
    raise ValueError('project must be a numeric GitLab project ID')
project = args.project
base = f'projects/{project}'
evidence = {}

def api(path, method='GET', body=None, env=None, allow_failure=False):
    args = ['glab', 'api', path, '--method', method]
    if body is not None:
        args += ['--input', '-', '-H', 'Content-Type: application/json']
    p = subprocess.run(args, input=None if body is None else json.dumps(body).encode(), env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    try:
        value = json.loads(p.stdout or b'{}')
    except ValueError:
        value = {}
    if p.returncode and (not allow_failure):
        raise RuntimeError(f"{method} {path}: {value.get('message', 'API request failed')}")
    return (p.returncode, value)
owner = api('user')[1]
if not owner['username'] == args.expected_user:
    raise ValueError('Unexpected signed-in GitLab account')
project_info = api(base)[1]
if not project_info['visibility'] == 'private':
    raise ValueError('Use a disposable private preflight project')
if args.evidence.exists():
    raise ValueError('Choose a new evidence path; do not overwrite prior test results')
token = None
pipeline = None
complete = False
try:
    expiry = (datetime.date.today() + datetime.timedelta(days=1)).isoformat()
    token = api(base + '/access_tokens', 'POST', {'name': 'neckbeard-disposable-gate-probe', 'scopes': ['api'], 'access_level': 40, 'expires_at': expiry})[1]
    bot_env = dict(os.environ, GITLAB_TOKEN=token['token'])
    bot = api('user', env=bot_env)[1]
    if not (bot['id'] == token['user_id'] and bot['id'] != owner['id']):
        raise ValueError('Approval identity assertion failed')
    evidence.update(project=project, token_id=token['id'], initiator_id=bot['id'], approver_id=owner['id'], token_expires=expiry)
    args.evidence.write_text(json.dumps(evidence, indent=2) + '\n')
    print('Temporary project bot created; separate initiator and approver verified.', flush=True)
    pipeline = api(base + '/pipeline', 'POST', {'ref': 'main'}, env=bot_env)[1]
    evidence['pipeline_id'] = pipeline['id']
    print('Identity-only preflight pipeline:', pipeline['id'], flush=True)
    deadline = time.monotonic() + 600
    while True:
        jobs = api(base + f"/pipelines/{pipeline['id']}/jobs")[1]
        matches = [j for j in jobs if j['name'] == 'preflight-prd']
        deployments = api(base + '/deployments?environment=prd&order_by=id&sort=desc&per_page=20')[1]
        deployments = [d for d in deployments if matches and d.get('deployable', {}).get('id') == matches[0]['id']]
        if deployments:
            job = matches[0]
            deployment = deployments[0]
            break
        if time.monotonic() > deadline:
            raise TimeoutError('No production preflight deployment created')
        time.sleep(5)
    evidence.update(job_id=job['id'], deployment_id=deployment['id'])
    code, response = api(base + f"/jobs/{job['id']}/play", 'POST', env=bot_env, allow_failure=True)
    if not code != 0:
        raise ValueError('Production job started without approval')
    evidence['play_before_approval'] = 'denied'
    code, response = api(base + f"/deployments/{deployment['id']}/approval", 'POST', {'status': 'approved'}, env=bot_env, allow_failure=True)
    if not (code != 0 and 'own deployment' in str(response).lower()):
        raise ValueError('Expected explicit self-approval denial')
    evidence['self_approval'] = 'denied'
    api(base + f"/deployments/{deployment['id']}/approval", 'POST', {'status': 'approved'})
    evidence['separate_maintainer_approval'] = 'accepted'
    print('Unapproved execution and bot self-approval denied; signed-in Maintainer approval accepted.', flush=True)
    api(base + f"/jobs/{job['id']}/play", 'POST')
    deadline = time.monotonic() + 600
    while True:
        current = api(base + f"/jobs/{job['id']}")[1]
        if current['status'] == 'success':
            complete = True
            break
        if current['status'] in ['failed', 'canceled', 'skipped']:
            raise RuntimeError('Production identity preflight ' + current['status'])
        if time.monotonic() > deadline:
            raise TimeoutError('Production identity preflight did not complete')
        time.sleep(10)
    evidence['production_identity_preflight'] = 'passed'
    print('Production identity preflight passed after approval.', flush=True)
finally:
    try:
        if pipeline and (not complete):
            api(base + f"/pipelines/{pipeline['id']}/cancel", 'POST', allow_failure=True)
    finally:
        try:
            if token:
                api(base + f"/access_tokens/{token['id']}", 'DELETE')
                evidence['temporary_token'] = 'revoked'
                print('Temporary bot access revoked.', flush=True)
        finally:
            args.evidence.write_text(json.dumps(evidence, indent=2) + '\n')
