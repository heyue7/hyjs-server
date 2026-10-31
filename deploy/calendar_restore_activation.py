"""Apply the calendar repair with stopped writers and a verified pre-deploy backup."""
import json
import hashlib
import os
from pathlib import Path
import re
import signal
import sys
import tempfile
import time
import uuid

VIP10_PRODUCT = 'p3vip10_growth_local'
VIP10_SOURCE = '本地会员成长礼包；默认关闭；1100钻、每日一份为本地规则，原购买价格未恢复；原item_044→item_064宝石技能书，VIP10及真实确认测试订单资格'
FORMAL110_OPENING_MILLIS = 1793462400000
_REVISION = re.compile(r'(?:[0-9a-f]{40}|[0-9a-f]{64})')
_SHA256 = re.compile(r'[0-9a-f]{64}')
MAINTENANCE_SQL = 'SELECT maintenance FROM client_update_policy WHERE id=1;'
VIP10_CHECK_SQL = (
    "SELECT COUNT(*) FROM p3_vip_shop_products WHERE BINARY id=BINARY '" + VIP10_PRODUCT +
    "' AND enabled=1 AND min_v_ip=10 AND cost=1100 AND BINARY cost_resource=BINARY 'diamond'"
    " AND total_limit=0 AND BINARY reward=BINARY 'item_044,1' AND BINARY source=BINARY '" + VIP10_SOURCE + "';"
)


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate_formal110_arguments(args, *, remote=False):
    """The opt-in applies only to one exact, standalone existing deployment."""
    if not getattr(args, 'calendar_formal_110', False):
        return
    request = getattr(args, 'restore_activity_calendar', None)
    try:
        canonical = bool(request) and str(uuid.UUID(request)) == request
    except (ValueError, AttributeError, TypeError):
        canonical = False
    require(canonical, 'Formal 110 requires a canonical calendar request UUID')
    live = getattr(args, 'expected_live_revision', None)
    require(isinstance(live, str) and _REVISION.fullmatch(live), 'Formal 110 requires an exact live revision')
    prohibited = ('migrate_role_wallets', 'apply_growth74', 'apply_growth75', 'apply_activity77',
                  'apply_feedback78', 'apply_feedback79', 'apply_feedback80', 'apply_feedback81',
                  'apply_trial82', 'apply_feedback84', 'apply_feedback85', 'maintenance_redeploy',
                  'warm_rush_history_player')
    prohibited += ('initial_db', 'provision_ingress') if remote else ('initial_dump', 'provision', 'update_ingress', 'check_config')
    require(not any(getattr(args, name, None) for name in prohibited), 'Formal 110 requires a standalone calendar cutover')


def validate_formal110_source_constants(path):
    """Bind the SQL predicate to the reviewed Go release constants."""
    source = Path(path).read_text(encoding='utf-8')
    for name, expected in (('P3VIP10GrowthProduct', VIP10_PRODUCT), ('P3VIP10GrowthSource', VIP10_SOURCE)):
        matches = re.findall(r'^const\s+' + name + r'\s*=\s*"([^"\n]*)"\s*$', source, re.MULTILINE)
        require(matches == [expected], 'Formal 110 VIP10 source constants differ from candidate')


def digest(path):
    with Path(path).open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def private_write(path, value):
    """Atomically checkpoint only in the private owned evidence directory."""
    fd, temporary = tempfile.mkstemp(prefix='.calendar-', dir=path.parent)
    try:
        os.fchmod(fd, 0o600)
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            json.dump(value, stream, ensure_ascii=False, indent=2)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
        directory_fd = os.open(path.parent, os.O_RDONLY)
        try:
            os.fsync(directory_fd)
        finally:
            os.close(directory_fd)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def require_maintenance(mysql):
    require(mysql(MAINTENANCE_SQL).strip() == '1', 'Formal 110 requires existing maintenance')


def require_vip10_approved(mysql):
    require(mysql(VIP10_CHECK_SQL).strip() == '1', 'Formal 110 VIP10 product differs from the reviewed configuration')


def require_authenticated_backup(backup_dir, backup, expected_live_revision):
    require(isinstance(backup, dict) and backup.get('format') == 'HYJSBK02-AES256GCM-CHUNKED',
            'Formal 110 requires an authenticated backup')
    require(backup.get('revisionBefore') == expected_live_revision, 'Backup live revision differs')
    name, sha = backup.get('file', ''), backup.get('sha256', '')
    require(isinstance(name, str) and bool(name) and Path(name).name == name and
            isinstance(sha, str) and _SHA256.fullmatch(sha), 'Invalid backup identity')
    file = Path(backup_dir) / name
    require(file.is_file() and not file.is_symlink() and digest(file) == sha, 'Authenticated backup changed')


class Formal110CutoverGuard:
    """Keep the owned app stopped on interruption; never change maintenance."""
    def __init__(self, compose, mysql, run, backup_dir, revision, expected_live_revision, request_id):
        self.compose, self.mysql, self.run = compose, mysql, run
        self.backup_dir = Path(backup_dir)
        self.revision, self.expected_live_revision, self.request_id = revision, expected_live_revision, request_id
        self.complete = False
        self.manifest = None
        self.record = None

    def prepare(self):
        require(_REVISION.fullmatch(self.revision) and _REVISION.fullmatch(self.expected_live_revision),
                'Formal 110 requires exact release revisions')
        require(str(uuid.UUID(self.request_id)) == self.request_id, 'Invalid formal request UUID')
        require(self.backup_dir.is_dir() and not self.backup_dir.is_symlink(), 'Private backup directory required')
        directory = self.backup_dir / ('formal110-' + self.request_id + '-' + str(time.time_ns()))
        directory.mkdir(mode=0o700)
        self.manifest = directory / 'manifest.json'
        self.record = {'schema': 'formal110-cutover-v1', 'revision': self.revision,
                       'expectedLiveRevision': self.expected_live_revision, 'requestId': self.request_id,
                       'state': 'guarded', 'maintenanceRequired': True, 'backupBefore': None,
                       'applyMayHaveCommitted': False, 'applyConfirmed': False}
        # A retry must inherit commit uncertainty before any new precondition
        # can fail, including backup or VIP10 validation.
        prior_dir = self.backup_dir / ('calendar-' + self.request_id)
        if prior_dir.exists() or prior_dir.is_symlink():
            self.record['applyMayHaveCommitted'] = True
            self.record['priorAttempt'] = 'unverified'
            try:
                prior_file = prior_dir / 'manifest.json'
                require(prior_dir.is_dir() and not prior_dir.is_symlink() and prior_file.is_file() and
                        not prior_file.is_symlink(), 'Invalid prior calendar evidence')
                prior = json.loads(prior_file.read_text())
                require(prior.get('requestId') == self.request_id and prior.get('revision') == self.revision and
                        prior.get('formal110') is True, 'Prior calendar identity differs')
                self.record['priorAttempt'] = prior.get('status', 'unknown')
                self.record['applyMayHaveCommitted'] = prior.get('applyMayHaveCommitted') is not False
                result_file = prior_dir / 'result.json'
                if prior.get('status') == 'applied' and result_file.is_file() and not result_file.is_symlink() and \
                        digest(result_file) == prior.get('resultSHA256'):
                    self.record['applyConfirmed'] = True
            except (OSError, ValueError, TypeError, AttributeError):
                pass
        self.checkpoint()

    def checkpoint(self, state=None):
        if state is not None:
            self.record['state'] = state
        private_write(self.manifest, self.record)

    def install_signal_handlers(self):
        def interrupted(signum, _frame):
            raise SystemExit('Formal 110 interrupted by signal ' + str(signum))
        for signum in (signal.SIGHUP, signal.SIGTERM, signal.SIGINT):
            signal.signal(signum, interrupted)

    def require_stopped(self):
        status = self.run('docker', 'inspect', '--format={{.State.Running}}', 'hyjs-game').stdout.decode().strip()
        require(status == 'false', 'Formal 110 requires the owned application to be stopped')

    def stop_if_unfinished(self):
        if self.complete:
            return
        stopped = False
        try:
            self.compose('stop', '-t', '30', 'app')
            self.require_stopped()
            stopped = True
        except Exception:
            pass
        if not stopped:
            print('Formal 110 could not confirm the app is stopped; inspect the target immediately.', file=sys.stderr)
        maintenance = 'unknown'
        try:
            observed = self.mysql(MAINTENANCE_SQL).strip()
            if observed in ('0', '1'):
                maintenance = observed
        except Exception:
            pass
        if self.record is not None:
            self.record['failureStage'] = self.record.get('state', 'guard-uninitialized')
            self.record['state'] = 'failed'
            self.record['stopConfirmed'] = stopped
            self.record['maintenanceObserved'] = maintenance
            self.record['recovery'] = ('Keep app stopped and maintenance unchanged; retain backups and inspect the request, '
                                       'because a calendar transaction may already have committed.')
            try:
                self.checkpoint()
            except Exception:
                print('Formal 110 failure evidence could not be saved; inspect the target immediately.', file=sys.stderr)


def command_json(result):
    output = result.stdout.decode() if isinstance(result.stdout, bytes) else result.stdout
    for line in reversed(output.splitlines()):
        try:
            value = json.loads(line)
        except (ValueError, TypeError):
            continue
        if isinstance(value, dict) and 'digest' in value:
            return value
    raise RuntimeError('Calendar CLI returned no complete JSON result')


def activate_formal110_calendar_restore(mysql, compose, backup_dir, revision, backup, request_id,
                                        expected_live_revision, guard):
    require(guard is not None and guard.record is not None, 'Formal 110 cutover guard is required')
    require(guard.request_id == request_id and guard.revision == revision and
            guard.expected_live_revision == expected_live_revision, 'Formal 110 guard identity differs')
    guard.checkpoint('checking-backup-and-vip10')
    guard.require_stopped()
    require_authenticated_backup(backup_dir, backup, expected_live_revision)
    require_maintenance(mysql)
    require_vip10_approved(mysql)
    record_dir = Path(backup_dir) / ('calendar-' + request_id)
    record_dir.mkdir(mode=0o700, exist_ok=True)
    require(record_dir.is_dir() and not record_dir.is_symlink() and not record_dir.stat().st_mode & 0o077,
            'Private calendar evidence directory required')
    record = record_dir / 'manifest.json'
    preview_file, result_file = record_dir / 'preview.json', record_dir / 'result.json'
    if record.exists():
        require(record.is_file() and not record.is_symlink(), 'Invalid calendar evidence file')
        manifest = json.loads(record.read_text())
        require(manifest.get('requestId') == request_id and manifest.get('revision') == revision and
                manifest.get('expectedLiveRevision') == expected_live_revision and manifest.get('formal110') is True,
                'Formal 110 retry identity differs')
        require_authenticated_backup(backup_dir, manifest.get('backup'), expected_live_revision)
        previous = manifest.setdefault('retryBackups', [])
        if backup != manifest['backup'] and backup not in previous:
            previous.append(backup)
    else:
        manifest = {'requestId': request_id, 'revision': revision, 'expectedLiveRevision': expected_live_revision,
                    'formal110': True, 'backup': backup, 'status': 'starting', 'applyMayHaveCommitted': False}
    private_write(record, manifest)
    common = ('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1',
              '--tables', '/app/tables', '--calendar-formal-110')
    guard.checkpoint('previewing')
    if manifest.get('digest'):
        real_millis = manifest.get('realMillis')
        require(type(real_millis) is int and real_millis > 0 and preview_file.is_file() and
                not preview_file.is_symlink() and digest(preview_file) == manifest.get('previewSHA256'),
                'Formal 110 retry preview differs')
        preview = json.loads(preview_file.read_text())
        require(preview.get('digest') == manifest['digest'], 'Formal 110 retry digest differs')
    else:
        real_millis = int(time.time() * 1000)
        preview = command_json(compose(*common, '--preview-activity-calendar', '--calendar-real-millis',
                                       str(real_millis), '--log-file', '/app/logs/calendar-preview.log'))
        private_write(preview_file, preview)
    require(preview.get('formal110') is True and preview.get('serverId') == 1 and
            preview.get('openingAt') == FORMAL110_OPENING_MILLIS and
            isinstance(preview.get('digest'), str) and _SHA256.fullmatch(preview['digest']) and
            isinstance(preview.get('windows'), list) and bool(preview['windows']),
            'Formal 110 preview identity, date or digest differs')
    require(isinstance(preview.get('blockers'), list) and not preview['blockers'],
            'Formal 110 preview has blockers; inspect private preview evidence')
    manifest.update(status='previewed', digest=preview['digest'], realMillis=real_millis,
                    previewSHA256=digest(preview_file))
    private_write(record, manifest)
    guard.record['previewDigest'] = preview['digest']
    guard.checkpoint('checking-before-apply')
    guard.require_stopped()
    require_authenticated_backup(backup_dir, backup, expected_live_revision)
    require_maintenance(mysql)
    require_vip10_approved(mysql)
    # Commit may happen even if the caller loses its response. Persist that fact
    # before invoking the CLI, and retry the same request/digest after inspection.
    manifest.update(status='applying', applyMayHaveCommitted=True)
    private_write(record, manifest)
    guard.record['applyMayHaveCommitted'] = True
    guard.checkpoint('applying-calendar')
    result = command_json(compose(*common, '--restore-activity-calendar', request_id,
                                  '--calendar-digest', preview['digest'], '--calendar-real-millis',
                                  str(real_millis), '--log-file', '/app/logs/calendar-apply.log'))
    private_write(result_file, result)
    clock = result.get('clock')
    require(result.get('formal110') is True and result.get('requestId') == request_id and
            result.get('digest') == preview['digest'] and result.get('openingAt') == preview['openingAt'] and
            type(result.get('windows')) is int and result['windows'] == len(preview['windows']) and
            isinstance(clock, dict) and clock.get('serverId') == 1 and type(clock.get('offsetDays')) is int and
            clock['offsetDays'] == 0, 'Formal 110 result identity, digest or clock differs')
    guard.record['applyConfirmed'] = True
    guard.checkpoint('verifying-applied-calendar')
    require(mysql('SELECT offset_days FROM server_game_clocks WHERE server_id=1;').strip() == '0',
            'Formal 110 clock offset differs')
    require_maintenance(mysql)
    # The reviewed product is retained; no restore CLI or product mutation runs.
    require_vip10_approved(mysql)
    manifest.update(status='applied', result=result, resultSHA256=digest(result_file))
    private_write(record, manifest)
    guard.checkpoint('calendar-applied')
    return {'requestId': request_id, 'digest': preview['digest'], 'openingAt': preview['openingAt'],
            'formal110': True, 'record': str(record.relative_to(Path(backup_dir).parent))}


def activate_calendar_restore(mysql, compose, backup_dir, revision, backup, request_id, *,
                              formal110=False, expected_live_revision=None, guard=None):
    if formal110:
        return activate_formal110_calendar_restore(mysql, compose, backup_dir, revision, backup, request_id,
                                                   expected_live_revision, guard)
    assert str(uuid.UUID(request_id)) == request_id, 'Calendar request must be a UUID'
    assert backup and backup.get('sha256'), 'Authenticated pre-deploy backup required'
    assert mysql('SELECT maintenance FROM client_update_policy WHERE id=1;').strip() == '1'
    record_dir = backup_dir / ('calendar-' + request_id)
    record_dir.mkdir(mode=0o700, exist_ok=True)
    record = record_dir / 'manifest.json'
    manifest = json.loads(record.read_text()) if record.exists() else {'requestId': request_id, 'revision': revision, 'backup': backup, 'status': 'previewing'}
    assert manifest['requestId'] == request_id and manifest['revision'] == revision, 'Calendar retry source changed'
    common = ('run', '--rm', '--no-deps', '-T', 'app', '--env', 'test', '--server-id', '1', '--tables', '/app/tables')
    if manifest.get('digest'):
        real_millis = manifest['realMillis']
        preview = json.loads((record_dir / 'preview.json').read_text())
        assert preview['digest'] == manifest['digest']
    else:
        real_millis = int(time.time() * 1000)
        preview = command_json(compose(*common, '--preview-activity-calendar', '--calendar-real-millis', str(real_millis), '--log-file', '/app/logs/calendar-preview.log'))
        (record_dir / 'preview.json').write_text(json.dumps(preview, ensure_ascii=False, indent=2))
    assert preview['serverId'] == 1 and preview['digest'] and preview['windows'], 'Empty or foreign calendar candidate'
    assert not preview.get('blockers'), 'Calendar preview blockers: ' + '; '.join(preview.get('blockers', []))
    manifest.update(status='applying', digest=preview['digest'], realMillis=real_millis)
    record.write_text(json.dumps(manifest, indent=2))
    result = command_json(compose(*common, '--restore-activity-calendar', request_id, '--calendar-digest', preview['digest'], '--calendar-real-millis', str(real_millis), '--log-file', '/app/logs/calendar-apply.log'))
    (record_dir / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2))
    assert result['requestId'] == request_id and result['digest'] == preview['digest'], 'Calendar result differs from preview'
    assert mysql('SELECT offset_days FROM server_game_clocks WHERE server_id=1;').strip() == '0'
    compose(*common, '--restore-vip10-book', '--log-file', '/app/logs/vip10-restore.log')
    assert mysql("SELECT enabled FROM p3_vip_shop_products WHERE id='p3vip10_growth_local';").strip() == '1'
    manifest.update(status='applied', result=result)
    record.write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
    return {'requestId': request_id, 'digest': preview['digest'], 'openingAt': preview['openingAt'], 'record': str(record.relative_to(backup_dir.parent))}
