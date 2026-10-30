"""Run fixed feedback84 tests serially in separately managed MySQL allocations.

The test binary must already be built from the fixed candidate. No database is
created until the shared lease, binary, selectors and fixture pins pass.
"""
import argparse
import fcntl
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

BINARY = ROOT / 'build/feedback84/service.test'
OUTPUT_ROOT = ROOT / 'analysis/feedback84/mysql'
FIXTURES = Path('/Users/lirui/LR/hyjs/analysis')
ROSTER = FIXTURES / 'p7/party-rush-isolated-roster-r2-20261016.json'
ROSTER_SHA256 = 'e60e9774a43efe58f67b9eb84c1a30cf40a3a30f2e5f68c87d063d3aeb29a5c9'
CASES = [
    ('publication', 'p4', 'TestFeedback84ExplicitSweepPublicationAndColdIdempotency'),
    ('publication', 'p4', 'TestFeedback84PublicationRejectsDriftAndNonMaintenance'),
    ('publication', 'p4', 'TestFeedback84PublicationRejectsInvalidOrStaleClock'),
    ('plunder', 'p11', 'TestP11Plunder84RealHolderPriorityRealmAndNoFakeProfiles'),
    ('plunder', 'p11', 'TestP11Plunder84ScanLimitNeverDeclaresUnknownEmpty'),
    ('plunder', 'p11', 'TestP11Plunder84AutoEnergyUsesOwnedMedicineAndColdReceipt'),
    ('plunder', 'p11', 'TestP11Plunder84NPCFightAndSweepUseFrozenSourceAndColdReceipts'),
    ('plunder', 'p11', 'TestP11Plunder84MatchingChecksCurrentTeamAcrossSameMillisecond'),
    ('breakthrough', 'p1', 'TestWarriorBreach84TransactionAndColdReceipt'),
    ('breakthrough', 'p1', 'TestWarriorBreach84CarryAndTerminalCosts'),
    ('breakthrough', 'p1', 'TestWarriorBreach84DonorInvestmentAndReset'),
    ('breakthrough', 'p1', 'TestWarriorBreach84MainResetRetiresPity'),
    ('wanted', 'p7', 'TestP12Wanted84RejectedRecordingAndUnknownReceiptRecovery'),
    ('wanted', 'p7', 'TestP12Wanted84LateRejectedEndKeepsRecoveredLease'),
    ('wanted', 'p7', 'TestP12Wanted84CommittedManualReceiptCannotMasqueradeAsAutomatic'),
    ('wanted', 'p7', 'TestP12Wanted86NativeRoundControlsStrictRecord'),
    ('wanted', 'p7', 'TestP12Wanted65ManualAutoEndColdConflict'),
    ('wanted', 'p7', 'TestP12Wanted84MySQLRecoveryCompetesWithEndWithoutNewCost'),
    ('wanted', 'p7', 'TestP12Wanted84MySQLRecoveryCompetesWithOriginalAwakeEntry'),
    ('arena', 'p11', 'TestSweep84ArenaRefreshTouchesOnlyParticipantAndChangedSnapshot'),
    ('arena', 'p11', 'TestSweep84ArenaParticipantSQLBudget'),
    ('arena', 'p11', 'TestSweep84ArenaComputationCrossesDailyCutoff'),
    ('arena', 'p11', 'TestP11ArenaScopedRealWinnerPaymentRanksConcurrentColdAndQualification'),
    ('arena', 'p11', 'TestP11ArenaVirtualOriginalGuideFightSlotConflictAndColdReceipt'),
    ('copy', 'p1', 'TestSweep84CopyActualSweepAndColdReceipt'),
]


def digest(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def table_digest():
    tables = ROOT / 'analysis/tables'
    files = sorted(tables.glob('*.tsv'))
    if not files:
        raise RuntimeError('Worktree static table directory is empty')
    value = hashlib.sha256()
    for path in files:
        value.update(path.name.encode('utf-8') + b'\0')
        value.update(bytes.fromhex(digest(path)))
    return {'count': len(files), 'sha256': value.hexdigest()}


def test_environment(connection, prefix):
    # Never inherit credentials or a developer/remote database selection.
    keep = {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TZ', 'SSL_CERT_FILE', 'SSL_CERT_DIR'}
    env = {key: value for key, value in os.environ.items() if key in keep}
    env.update({'GOMAXPROCS': '2', 'GOMEMLIMIT': '768MiB', 'HYJS_TEST_FIXTURE_ROOT': str(FIXTURES)})
    values = {'HOST': connection.host, 'USER': connection.user, 'PASSWORD': connection.password,
              'DATABASE': connection.database}
    for key, value in values.items():
        env['HYJS_' + prefix.upper() + '_MYSQL_' + key] = value
    return env


def run_test(name, env):
    command = ['rtk', 'proxy', str(BINARY), '-test.v', '-test.run', '^' + re.escape(name) + '$',
               '-test.count=1', '-test.timeout=8m']
    process = subprocess.Popen(command, cwd=ROOT / 'hyjs-server/service', env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                               start_new_session=True)
    try:
        output, _ = process.communicate(timeout=490)
        return process.returncode, output.decode(errors='replace'), False
    except BaseException as error:
        if process.poll() is None:
            try:
                os.killpg(process.pid, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                output, _ = process.communicate(timeout=10)
            except subprocess.TimeoutExpired:
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                output, _ = process.communicate()
        else:
            output, _ = process.communicate()
        if isinstance(error, subprocess.TimeoutExpired):
            return None, output.decode(errors='replace') + '\nTEST PROCESS TIMED OUT\n', True
        raise


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--only', action='append', choices=[name for _, _, name in CASES])
    parser.add_argument('--run-id', required=True, help='Fresh evidence directory name under analysis/feedback84/mysql')
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z0-9][a-z0-9-]{0,63}', args.run_id):
        parser.error('Use a short lowercase run id')
    cases = [case for case in CASES if not args.only or case[2] in args.only]
    output_dir = OUTPUT_ROOT / args.run_id
    if output_dir.exists():
        raise SystemExit('Evidence directory already exists; use a fresh run id')
    if not BINARY.is_file() or not FIXTURES.is_dir():
        raise SystemExit('Prebuilt binary or required original fixture is missing')
    if not ROSTER.is_file() or digest(ROSTER) != ROSTER_SHA256:
        raise SystemExit('Original equipped roster SHA differs')
    binary_sha, tables = digest(BINARY), table_digest()
    keep = {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL', 'TZ', 'SSL_CERT_FILE', 'SSL_CERT_DIR'}
    listing_env = {key: value for key, value in os.environ.items() if key in keep}
    listing = subprocess.run(['rtk', 'proxy', str(BINARY), '-test.list', '^Test'],
                             cwd=ROOT / 'hyjs-server/service', capture_output=True, text=True,
                             env=listing_env, check=True, timeout=30)
    available = set(listing.stdout.splitlines())
    missing = [name for _, _, name in cases if name not in available]
    if missing:
        raise SystemExit('Compiled test binary is missing cases: ' + ', '.join(missing))

    # Held across every case, including release. Other shared instances are untouched.
    with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lease:
        try:
            fcntl.flock(lease, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            print('Shared MySQL lease held elsewhere; no allocations or tests started.')
            return 75
        output_dir.mkdir(parents=True, exist_ok=False)
        started = time.monotonic()
        results = []
        report = {'binarySHA256': binary_sha, 'tableFixture': tables, 'rosterSHA256': ROSTER_SHA256,
                  'expectedCases': [name for _, _, name in cases],
                  'results': results, 'ownedAllocationsReleased': False, 'pass': False}

        def checkpoint():
            report['elapsedSeconds'] = round(time.monotonic() - started, 3)
            (output_dir / 'result.json').write_text(json.dumps(report, indent=2) + '\n')

        def interrupted(signum, _frame):
            raise SystemExit('Interrupted by signal ' + str(signum))

        for signum in (signal.SIGHUP, signal.SIGTERM):
            signal.signal(signum, interrupted)
        try:
            for index, (group, prefix, name) in enumerate(cases):
                if digest(BINARY) != binary_sha or table_digest() != tables or digest(ROSTER) != ROSTER_SHA256:
                    raise RuntimeError('Binary or required fixture changed during the run')
                pool = ManagedMySQL()  # A new pool gives each case its own temporary account.
                connection = None
                record = {'test': name, 'group': group, 'fixture': prefix, 'status': 'ERROR', 'cleanup': False}
                try:
                    label = prefix + '_feedback84_' + str(index)
                    connection = pool.allocate(label)
                    if not connection.database.startswith('hyjs_test_' + prefix + '_'):
                        raise RuntimeError('Allocated schema does not match the fixture prefix')
                    began = time.monotonic()
                    code, output, timed_out = run_test(name, test_environment(connection, prefix))
                    record['elapsedSeconds'] = round(time.monotonic() - began, 3)
                    record['exitCode'] = code
                    record['timedOut'] = timed_out
                    output = pool.redact(output).replace(connection.user, '[test-user]')
                    (output_dir / (f'{index:02d}-' + name + '.log')).write_text(output)
                    db_row = pool.sql(connection.database, "SELECT DATABASE(),ENGINE FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='profiles';").strip().split('\t')
                    record['databaseEngine'] = db_row[1] if len(db_row) == 2 else 'missing'
                    record['ownedSchemaConfirmed'] = len(db_row) == 2 and db_row[0] == connection.database and db_row[1] == 'InnoDB'
                    skipped = '--- SKIP:' in output or '[no tests to run]' in output
                    exact_pass = re.search(r'^--- PASS: ' + re.escape(name) + r'(?:\s|$)', output, re.M) is not None
                    passed = code == 0 and exact_pass and not skipped and record['ownedSchemaConfirmed']
                    record['status'] = 'PASS' if passed else ('SKIP' if skipped else 'FAIL')
                except Exception as error:
                    record['errorType'] = type(error).__name__
                finally:
                    database = connection.database if connection is not None else None
                    try:
                        pool.close()
                        record['cleanup'] = not pool.connections and (database is None or
                            not (SERVICES / 'test-connections' / (database + '.env')).exists() and
                            not (SERVICES / 'test-connections' / (database + '.json')).exists())
                    except Exception as error:
                        record['cleanupErrorType'] = type(error).__name__
                    results.append(record)
                    print(json.dumps(record), flush=True)
                    checkpoint()
                if not record['cleanup']:
                    raise RuntimeError('Managed cleanup failed; later cases were not run')
        finally:
            report['ownedAllocationsReleased'] = all(row.get('cleanup') for row in results)
            try:
                report['binaryUnchanged'] = BINARY.is_file() and digest(BINARY) == binary_sha
                report['tableFixtureUnchanged'] = table_digest() == tables
            except (OSError, RuntimeError):
                report['binaryUnchanged'] = False
                report['tableFixtureUnchanged'] = False
            report['pass'] = (len(results) == len(cases) and all(row['status'] == 'PASS' and row['cleanup'] for row in results)
                              and report['binaryUnchanged'] and report['tableFixtureUnchanged'])
            checkpoint()
            fcntl.flock(lease, fcntl.LOCK_UN)
    return 0 if report['pass'] else 1


if __name__ == '__main__':
    sys.exit(main())
