"""Local hyjs project access to the allocated shared MySQL service.

Credentials stay in the central 0600 project file and process stdin/environment.
Test provisioning is owned by the central manage.py interface.
"""
import os
from pathlib import Path
import re
import subprocess

SERVICES = (Path.home() / '.codex/local-services')
PROJECT = 'hyjs'


def settings():
    if not (SERVICES / 'README.md').is_file():
        raise RuntimeError('Read the local-services allocation before starting local services')
    path = SERVICES / 'projects/hyjs.env'
    if path.stat().st_mode & 0o077:
        raise RuntimeError('The allocated credential file must remain private')
    data = dict(line.split('=', 1) for line in path.read_text().splitlines()
                if '=' in line and not line.startswith('#'))
    expected = {'MYSQL_HOST': '127.0.0.1', 'MYSQL_PORT': '3306',
                'MYSQL_DATABASE': 'hyjs', 'MYSQL_USER': 'hyjs',
                'MYSQL_TEST_PREFIX': 'hyjs_test_', 'REDIS_DATABASE': '1',
                'REDIS_USERNAME': 'hyjs', 'REDIS_KEY_PREFIX': 'hyjs:'}
    if any(data.get(key) != value for key, value in expected.items()):
        raise RuntimeError('The hyjs shared-service allocation changed; inspect its README')
    if not data.get('MYSQL_PASSWORD') or '\n' in data['MYSQL_PASSWORD']:
        raise RuntimeError('Missing allocated project password')
    return data


def environment():
    data = settings()
    env = dict(os.environ)
    env.update(HYJS_MYSQL_HOST=data['MYSQL_HOST'] + ':' + data['MYSQL_PORT'],
               HYJS_MYSQL_USER=data['MYSQL_USER'], HYJS_MYSQL_DATABASE=data['MYSQL_DATABASE'],
               HYJS_MYSQL_PASSWORD=data['MYSQL_PASSWORD'])
    return env


def sql(query, database='hyjs'):
    data = settings()
    if database != data['MYSQL_DATABASE']:
        raise RuntimeError('Use the managed test connection for an isolated test database')
    if re.search(r'\b(?:CREATE|DROP)\s+USER\b|\b(?:FLUSH|GRANT|REVOKE)\b', query, re.I):
        raise RuntimeError('Global administration is outside project SQL access')
    script = ('read -r MYSQL_PWD; export MYSQL_PWD; '
              'exec mysql -h127.0.0.1 -u "$1" --default-character-set=utf8mb4 '
              '--binary-mode --batch --raw --skip-column-names "$2"')
    result = subprocess.run(
        ['rtk', 'proxy', 'docker', 'exec', '-i', 'codex-local-mysql', 'sh', '-c',
         script, '--', data['MYSQL_USER'], database],
        input=data['MYSQL_PASSWORD'] + '\n' + query, text=True,
        capture_output=True, timeout=30)
    if result.returncode:
        raise RuntimeError('Allocated hyjs MySQL query failed; no credentials were logged')
    return result.stdout


def dump(output, database='hyjs'):
    data = settings()
    if database != data['MYSQL_DATABASE']:
        raise RuntimeError('Local release backup must use the allocated development database')
    script = ('read -r MYSQL_PWD; export MYSQL_PWD; '
              'exec mysqldump -h127.0.0.1 -u "$1" --single-transaction '
              '--no-tablespaces --set-gtid-purged=OFF --routines --triggers "$2"')
    result = subprocess.run(
        ['rtk', 'proxy', 'docker', 'exec', '-i', 'codex-local-mysql', 'sh', '-c',
         script, '--', data['MYSQL_USER'], database],
        input=(data['MYSQL_PASSWORD'] + '\n').encode(), stdout=output,
        stderr=subprocess.PIPE, timeout=90)
    if result.returncode:
        raise RuntimeError('Allocated hyjs MySQL backup failed; server remains unchanged')
