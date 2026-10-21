"""Run client-update persistence checks against an owned shared-MySQL allocation."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

from hyjs_managed_mysql import ManagedMySQL

ROOT = Path(__file__).resolve().parents[1]


def main():
    pool = ManagedMySQL()
    started = time.monotonic()
    result = {"status": "RUNNING", "scope": "isolated MySQL client update business suite"}
    try:
        connection = pool.allocate('client_updates')
        dsn = f'{connection.user}:{connection.password}@tcp({connection.host})/{connection.database}?charset=utf8mb4&parseTime=True&loc=UTC'
        environment = dict(os.environ, HYJS_CLIENT_UPDATE_MYSQL_DSN=dsn)
        run = subprocess.run(['rtk', 'proxy', 'go', 'test', '-p', '1', './service', '-run', '^TestClientUpdateMySQL$', '-count=1'], cwd=ROOT / 'hyjs-server', env=environment, text=True, capture_output=True, timeout=300)
        output = pool.redact(run.stdout + run.stderr)
        print(output[-6000:])
        result.update(status='PASS' if run.returncode == 0 else 'FAIL', exitCode=run.returncode)
    finally:
        pool.close()
        result.update(ownedDatabaseAndAccountReleased=True, seconds=round(time.monotonic() - started, 3))
        (ROOT / 'analysis/client-update-20261021/mysql.json').write_text(json.dumps(result, indent=2) + '\n')
    return 0 if result['status'] == 'PASS' else 1


if __name__ == '__main__':
    sys.exit(main())
