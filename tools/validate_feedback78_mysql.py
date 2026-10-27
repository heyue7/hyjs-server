"""Run 78 representatives serially against separately owned managed MySQL schemas."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL, SERVICES

BINARY = ROOT / 'build/feedback78/service.test'
OUTPUT_ROOT = ROOT / 'analysis/feedback78'
FIXTURES = Path('/Users/lirui/LR/hyjs/analysis')
CASES = [
    ('activity', 'p1', 'TestActivityRewardIsolation78ActualPeriodsAndCenter'),
    ('activity', 'p1', 'TestRechargeClock75PublicationGateAndHistoricalClaimPreservation'),
    ('activity', 'p4', 'TestActivity77PeriodUnclaimedDailyReissued'),
    ('activity', 'p1', 'TestPeriodRewardNoticeHistoryBothListsPreserveReceipt'),
    ('reward', 'p1', 'TestP1OfflineRewardConcurrencyReplayAndRestart'),
    ('reward', 'p1', 'TestP1BatchOwnershipExpiryOverflowAndRollback'),
    ('trial', 'p7', 'TestP7ReRushMySQLPublishKeepsOldPendingFightSectionRewardAndBuff'),
    ('trial', 'p7', 'TestP7RushAdditionCertifiedScopedLegacyPurchaseKeepsFrozenSources'),
    ('sprout', 'p10', 'TestSproutStatus78AdminSettingsSceneAndSession'),
    ('guild', 'p13', 'TestP13GuildDonationBadgePaidSourceMailClaimThenOriginalDonateBox'),
    ('config', 'p1', 'TestFeedback78ConfigPreservesHistoryAndIsIdempotent'),
    ('config', 'p1', 'TestFeedback78ConfigRejectsDriftWithoutPartialWrites'),
    ('config', 'p1', 'TestFeedback78ExpiryUsesRealmClock'),
    ('config', 'p1', 'TestFeedback78PreflightRejectsOversize'),
]


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def test_environment(connection, prefix):
    # Never inherit a developer or remote database credential into the test process.
    keep = {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TZ', 'SSL_CERT_FILE', 'SSL_CERT_DIR'}
    env = {key: value for key, value in os.environ.items() if key in keep}
    env.update({'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB', 'HYJS_TEST_FIXTURE_ROOT': str(FIXTURES)})
    for key, value in {'HOST': connection.host, 'USER': connection.user, 'PASSWORD': connection.password,
                       'DATABASE': connection.database}.items():
        env['HYJS_' + prefix.upper() + '_MYSQL_' + key] = value
    return env


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--only', action='append', choices=[name for _, _, name in CASES])
    parser.add_argument('--batch', choices=['mysql-r1', 'mysql-r2'], default='mysql-r1')
    parser.add_argument('--output-subdir')
    args = parser.parse_args()
    if args.output_subdir and not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,63}', args.output_subdir):
        parser.error('Use a short owned evidence subdirectory')
    batch_dir = OUTPUT_ROOT / args.batch
    output_dir = batch_dir / args.output_subdir if args.output_subdir else batch_dir
    cases = [case for case in CASES if not args.only or case[2] in args.only]
    if not BINARY.is_file() or not FIXTURES.is_dir():
        raise SystemExit('Required prebuilt test binary or immutable fixture directory is missing')
    binary_sha = digest(BINARY)
    listing = subprocess.run(['rtk', 'proxy', str(BINARY), '-test.list', '^Test'], cwd=ROOT / 'hyjs-server/service',
                             capture_output=True, text=True, check=True, timeout=20)
    available = set(listing.stdout.splitlines())
    missing = [name for _, _, name in cases if name not in available]
    if missing:
        raise SystemExit('Compiled test binary is missing requested cases: ' + ', '.join(missing))
    output_dir.mkdir(parents=True, exist_ok=False)
    pool = ManagedMySQL()
    results, blocked = [], set()
    started = time.monotonic()
    report = {'binarySHA256': binary_sha, 'expectedCases': [name for _, _, name in cases], 'results': results,
              'ownedAllocationsReleased': False, 'pass': False}

    def checkpoint():
        report['elapsedSeconds'] = round(time.monotonic() - started, 3)
        (output_dir / 'result.json').write_text(json.dumps(report, indent=2) + '\n')

    def interrupt(signum, _frame):
        raise SystemExit('Interrupted by signal ' + str(signum))

    for signum in (signal.SIGHUP, signal.SIGTERM):
        signal.signal(signum, interrupt)
    try:
        for index, (group, prefix, name) in enumerate(cases):
            if group in blocked:
                record = {'test': name, 'group': group, 'status': 'NOT_RUN_GROUP_FAILED'}
                results.append(record)
                print(json.dumps(record), flush=True)
                checkpoint()
                continue
            if digest(BINARY) != binary_sha:
                raise RuntimeError('Prebuilt test binary changed during the run')
            connection = None
            record = {'test': name, 'group': group, 'fixture': prefix, 'status': 'ERROR', 'cleanup': False}
            try:
                label = prefix + ('_feedback78_' if group == 'config' else '_78_') + str(index)
                connection = pool.allocate(label)
                if not connection.database.startswith('hyjs_test_' + label + '_'):
                    raise RuntimeError('Managed schema does not match the fixture prefix')
                env = test_environment(connection, prefix)
                began = time.monotonic()
                try:
                    run = subprocess.run(['rtk', 'proxy', str(BINARY), '-test.v', '-test.run', '^' + re.escape(name) + '$',
                                          '-test.count=1', '-test.timeout=160s'], cwd=ROOT / 'hyjs-server/service',
                                         env=env, capture_output=True, text=True, timeout=165)
                    output = run.stdout + run.stderr
                    record['exitCode'] = run.returncode
                except subprocess.TimeoutExpired as error:
                    chunks = [error.stdout or b'', error.stderr or b'']
                    output = ''.join(chunk.decode(errors='replace') if isinstance(chunk, bytes) else chunk for chunk in chunks)
                    output += '\nTEST PROCESS TIMED OUT\n'
                    record['exitCode'] = None
                record['elapsedSeconds'] = round(time.monotonic() - began, 3)
                output = pool.redact(output).replace(connection.user, '[test-user]')
                (output_dir / (f'{index:02d}-' + name + '.log')).write_text(output)
                result = pool.sql(connection.database, "SELECT DATABASE(),ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='profiles';").strip().split('\t')
                record['databaseEngine'] = result[1] if len(result) == 2 else 'missing'
                record['ownedSchemaConfirmed'] = len(result) == 2 and result[0] == connection.database and result[1] == 'InnoDB'
                skipped = '--- SKIP:' in output or '[no tests to run]' in output
                passed = record['exitCode'] == 0 and '--- PASS: ' + name in output and not skipped and record['ownedSchemaConfirmed']
                record['status'] = 'PASS' if passed else ('SKIP' if skipped else 'FAIL')
                if not passed:
                    blocked.add(group)
            except Exception as error:
                blocked.add(group)
                record['errorType'] = type(error).__name__
            finally:
                if connection is not None:
                    try:
                        pool.release(connection.database)
                        env_file = SERVICES / 'test-connections' / (connection.database + '.env')
                        registry_file = SERVICES / 'test-connections' / (connection.database + '.json')
                        record['cleanup'] = not env_file.exists() and not registry_file.exists() and not pool.connections
                    except Exception as error:
                        record['cleanupErrorType'] = type(error).__name__
                results.append(record)
                print(json.dumps(record), flush=True)
                checkpoint()
                if not record['cleanup']:
                    raise RuntimeError('Managed test cleanup did not complete')
    finally:
        pool.close()
        report['ownedAllocationsReleased'] = not pool.connections
        report['binaryUnchanged'] = digest(BINARY) == binary_sha
        report['pass'] = len(results) == len(cases) and all(row['status'] == 'PASS' and row.get('cleanup') for row in results) and report['binaryUnchanged']
        checkpoint()
    if not report['pass']:
        raise SystemExit(1)


if __name__ == '__main__':
    main()
