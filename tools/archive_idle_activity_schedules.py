#!/usr/bin/env python3
"""Review or archive exact stopped activity slots with whole-calendar version protection."""
import argparse
import copy
import http.cookiejar
import json
import urllib.request
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-url', required=True)
    parser.add_argument('--credentials-file', type=Path, required=True)
    parser.add_argument('--server-id', type=int, required=True)
    parser.add_argument('--id', action='append', required=True)
    parser.add_argument('--expected-version')
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.apply and (not args.expected_version or len(args.expected_version) != 64):
        parser.error('--apply requires the reviewed --expected-version')
    base = args.base_url.rstrip('/') + '/admin/api'
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
    csrf = ''

    def call(method, path, data=None):
        headers = {'Content-Type': 'application/json'}
        if csrf and method != 'GET':
            headers['X-CSRF-Token'] = csrf
        request = urllib.request.Request(base + path, method=method, headers=headers,
                                         data=None if data is None else json.dumps(data).encode())
        with opener.open(request, timeout=30) as response:
            result = json.load(response)
        if result.get('code') != 0:
            raise RuntimeError('Activity operation rejected; credentials and response omitted')
        return result['data']

    login = call('POST', '/auth/login', json.loads(args.credentials_file.read_text()))
    csrf = login['csrfToken']
    try:
        path = f'/activity-schedules?serverId={args.server_id}'
        before = call('GET', path)
        ids = set(args.id)
        matches = [row for row in before['items'] if row['id'] in ids]
        if len(matches) != len(ids) or any(row['enabled'] for row in matches):
            raise RuntimeError('Every requested slot must exist and already be stopped')
        report = {'status': 'review', 'before': before, 'archiveIds': sorted(ids),
                  'historyPreserved': True, 'after': None}
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2))
        if args.apply:
            if before['version'] != args.expected_version:
                raise RuntimeError('Calendar changed since review')
            body = {'version': before['version'], 'items': [copy.deepcopy(row) for row in before['items'] if row['id'] not in ids]}
            preview = call('POST', f'/activity-schedules/preview?serverId={args.server_id}', body)
            if not preview['valid']:
                raise RuntimeError('Archive preview rejected; no calendar mutation')
            saved = call('PUT', path, body)
            after = call('GET', path)
            def business(row):
                return {key: value for key, value in row.items() if key not in ('version', 'frozen')}
            expected = {row['id']: business(row) for row in body['items']}
            actual = {row['id']: business(row) for row in after['items']}
            if expected != actual or after['version'] != saved['version']:
                raise RuntimeError('Read-back differs; retain report and investigate')
            report['status'] = 'archived'
            report['after'] = after
        args.output.parent.mkdir(parents=True, exist_ok=True)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2))
        print(json.dumps({'status': report['status'], 'archiveIds': sorted(ids),
                          'beforeCount': len(before['items']), 'afterCount': None if report['after'] is None else len(report['after']['items']),
                          'version': before['version']}, ensure_ascii=False))
    finally:
        call('POST', '/auth/logout', {})


if __name__ == '__main__':
    main()
