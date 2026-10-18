"""Qualify pinned incremental source against an already qualified exact base."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

from test_candidate import ROOT, run_phase


def digest(path):
    with path.open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--candidate', type=Path, required=True)
    parser.add_argument('--base-record', type=Path, required=True)
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--run', required=True)
    parser.add_argument('--keep-going', action='store_true',
                        help='Finish a small affected-test batch to collect independent failures before rebuilding')
    args = parser.parse_args()
    candidate, baseline, record, binary = [path.resolve() for path in
                                         (args.candidate, args.base_record, args.record, args.binary)]
    if (candidate.parent != ROOT / 'build/candidates'
            or not baseline.is_relative_to(ROOT / 'analysis/p3')
            or not record.is_relative_to(ROOT / 'analysis/p3') or record == baseline
            or record.exists() or binary.exists() or binary.parent != ROOT / 'build'
            or not re.fullmatch(r'hyjs-server-[a-z0-9-]+-pending', binary.name)):
        raise ValueError('Use a new owned candidate record and pending binary')
    manifest_path = candidate / 'manifest.json'
    manifest = json.loads(manifest_path.read_text())
    base = json.loads(baseline.read_text())
    if (manifest.get('stable') or not base.get('qualified')
            or base['candidate'] != manifest['fixedBase']):
        raise ValueError('Require an unqualified increment and its exact qualified base')

    def verify_pins():
        for row in manifest['files']:
            path = (candidate / row['path']).resolve()
            if not path.is_relative_to(candidate) or digest(path) != row['sha256']:
                raise RuntimeError('Pinned source changed: ' + row['path'])

    started = time.monotonic()
    verify_pins()
    environment = dict(os.environ, GOMEMLIMIT='768MiB', GOMAXPROCS='2', GOTOOLCHAIN='go1.27.2')
    command = ['go', 'test', '-json', *([] if args.keep_going else ['-failfast']), '-p=1', '-count=1', '-run', args.run,
               '-timeout', '5m', './...']
    phase_started = time.monotonic()
    code, rows, tail = run_phase(command, candidate / 'hyjs-server', environment, 'impacted')
    phases = [{'command': command, 'exitCode': code,
               'elapsedSeconds': round(time.monotonic() - phase_started, 3)}]
    if not code:
        for command in (['go', 'vet', '-p=1', './...'],
                        ['go', 'build', '-p=1', '-o', str(binary), '.']):
            phase_started = time.monotonic()
            result = subprocess.run(['rtk', 'proxy', *command], cwd=candidate / 'hyjs-server',
                                    env=environment, capture_output=True, text=True)
            code = result.returncode
            phases.append({'command': command, 'exitCode': code,
                           'elapsedSeconds': round(time.monotonic() - phase_started, 3)})
            if code:
                tail.append((result.stdout + result.stderr)[-6000:])
                break
    # Seal only source bytes checked both before and after this actual run.
    verify_pins()
    report = {'candidate': str(candidate.relative_to(ROOT)), 'qualified': code == 0,
              'mode': 'Impacted tests plus all-package compile/vet/build; exact qualified base reused; no full-suite rerun claim',
              'baseRecord': str(baseline.relative_to(ROOT)), 'baseRecordSHA256': digest(baseline),
              'pinnedFilesVerified': len(manifest['files']), 'results': rows, 'phases': phases,
              'elapsedSeconds': round(time.monotonic() - started, 3),
              'binary': str(binary.relative_to(ROOT)), 'binarySHA256': digest(binary) if not code else None,
              'failureTail': tail if code else []}
    record.parent.mkdir(parents=True, exist_ok=True)
    record.write_text(json.dumps(report, indent=2) + '\n')
    if not code:
        manifest.update(stable=True, qualification={'record': str(record.relative_to(ROOT)),
                                                    'sha256': digest(record), 'mode': 'incremental'})
        manifest_path.write_text(json.dumps(manifest, indent=2) + '\n')
    print(json.dumps({key: report[key] for key in
                      ('qualified', 'elapsedSeconds', 'pinnedFilesVerified', 'binarySHA256')}), flush=True)
    raise SystemExit(code)


if __name__ == '__main__':
    main()
