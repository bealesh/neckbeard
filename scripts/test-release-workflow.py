#!/usr/bin/env python3
"""Check the real publish step without contacting GitHub."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap

workflow = Path(__file__).resolve().parents[1] / ".github/workflows/release.yml"
publish = workflow.read_text().split("      - name: Publish GitHub Release\n", 1)[1]
script = textwrap.dedent(publish.split("        run: |\n", 1)[1])

with tempfile.TemporaryDirectory() as tmp:
    root = Path(tmp)
    mock = root / "gh"
    mock.write_text(
        "#!/usr/bin/env python3\n"
        "import json, os, sys\n"
        "with open(os.environ['CAPTURE'], 'w') as out:\n"
        "    json.dump(sys.argv[1:], out)\n"
    )
    mock.chmod(0o755)
    dist = root / "release assets"
    dist.mkdir()
    (dist / "checksums.txt").touch()
    (dist / "neckbeard.tar.gz").touch()
    capture = root / "args.json"
    for tag in ("v0.4.0", "v0.4.0-beta.1", "v0.4.0-rc.1"):
        env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                   DIST=str(dist), GITHUB_REF_NAME=tag, CAPTURE=str(capture))
        subprocess.run(["sh", "-c", script], env=env, check=True)
        args = json.loads(capture.read_text())
        assert args[:3] == ["release", "create", tag], args
        assert str(dist / "checksums.txt") in args, args
        assert str(dist / "neckbeard.tar.gz") in args, args
        assert ("--prerelease" in args) == ("-" in tag), args
        assert "--verify-tag" in args, args
        assert args[args.index("--title") + 1] == tag, args
print("Release publishing checks passed: stable, beta, and release candidate.")
