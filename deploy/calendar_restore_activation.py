"""Apply the calendar repair with stopped writers and a verified pre-deploy backup."""
import json
import time
import uuid


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


def activate_calendar_restore(mysql, compose, backup_dir, revision, backup, request_id):
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
