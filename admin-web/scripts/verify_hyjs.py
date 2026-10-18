#!/usr/bin/env python3
"""Build the adapted upstream frontend and pin the exact source and assets; no server, DB or browser mutation."""
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path(__file__).resolve().parents[1]
output = root / 'validation'
output.mkdir(exist_ok=True)
date = datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))
stamp = date.strftime('%Y%m%dT%H%M%S')
record = {'status': 'BUILD_TYPECHECK_PASSED_BROWSER_API_PENDING', 'time': date.isoformat(),
          'upstream': json.loads((root / 'UPSTREAM.json').read_text()), 'steps': [], 'apiBrowserVerified': False}
env = os.environ.copy()
env['COREPACK_ENABLE_PROJECT_SPEC'] = '0'
for name in ['typecheck', 'build']:
    started = time.monotonic()
    result = subprocess.run(['pnpm', 'run', name], cwd=root, env=env, text=True,
                            stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    log = f'{name}-{stamp}.log'
    (output / log).write_text(result.stdout)
    step = {'name': name, 'command': f'rtk proxy env COREPACK_ENABLE_PROJECT_SPEC=0 pnpm run {name}',
            'exitCode': result.returncode, 'elapsedSeconds': round(time.monotonic() - started, 3),
            'log': f'validation/{log}'}
    record['steps'].append(step)
    print(json.dumps(step), flush=True)
    if result.returncode:
        record['status'] = 'FAILED'
        break
paths = []
for directory in ['src', 'build', 'packages', 'dist', 'scripts']:
    paths.extend(p for p in (root / directory).rglob('*') if p.is_file() and 'node_modules' not in p.parts)
for name in ['package.json', 'pnpm-lock.yaml', 'UPSTREAM.json', 'UPSTREAM_ICONS.json', 'LICENSE', '.env', '.env.prod', '.env.test',
             'vite.config.ts', 'pnpm-workspace.yaml', 'README.HYJS.md']:
    paths.append(root / name)
record['files'] = {str(p.relative_to(root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(paths)}
(output / f'{stamp}-build.json').write_text(json.dumps(record, ensure_ascii=False, indent=2) + '\n')
print(json.dumps({'status': record['status'], 'pinnedFiles': len(record['files']),
                  'distBytes': sum(p.stat().st_size for p in (root / 'dist').rglob('*') if p.is_file())}), flush=True)
sys.exit(1 if record['status'] == 'FAILED' else 0)
