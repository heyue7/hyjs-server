"""Inspect or explicitly activate the two named local guild donation sources.

Default operation is read-only. Mutation touches only the two policy rows after
matching the running binary, qualified candidate, exact rule fingerprints and
current guild economy gates. It never edits profiles, membership, scores, mail,
inventory, quotas, original tables or other domain policies.
"""
import argparse
import fcntl
import hashlib
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.shared_mysql import sql
from tools.release_go import operations


def rules(source_root):
    result = {}
    for label, filename, prefix in [
        ('boxes', 'p13GuildDonateBox.go', 'P13GuildDonateBox'),
        ('badges', 'p13GuildDonationBadge.go', 'P13GuildDonationBadge'),
    ]:
        path = source_root / 'hyjs-server/constUtil' / filename
        text = path.read_text()
        values = {}
        for key in ['Version', 'Source']:
            match = re.search(r'\b' + prefix + key + r'\s*=\s*("[^\n]*")', text)
            if not match:
                raise RuntimeError('Named donation rule is absent: ' + label)
            values[key.lower()] = json.loads(match.group(1))
        values['sourceHash'] = hashlib.sha256(values.pop('source').encode()).hexdigest()
        if not re.fullmatch('[a-z0-9-]{1,128}', values['version']):
            raise RuntimeError('Policy version cannot be safely selected')
        values['constPath'] = str(path.relative_to(source_root))
        values['constSHA256'] = hashlib.sha256(path.read_bytes()).hexdigest()
        result[label] = values
    return result


def protected_fingerprints():
    return sql('SELECT id,SHA2(player,256),SHA2(game,256) FROM profiles '
               'WHERE id IN (1,4) ORDER BY id;').strip().splitlines()


def inspect(player_id=4):
    available = sql('SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE() '
                    'AND TABLE_NAME IN ("p13_guild_donate_box_policies",'
                    '"p13_guild_donation_badge_policies");').strip().splitlines()
    gate = sql('SELECT management_enabled,economy_enabled,chat_enabled FROM '
               'p13_guild_policies WHERE id=1;').strip().split('\t')
    policy = {}
    for key, table in [('boxes', 'p13_guild_donate_box_policies'),
                       ('badges', 'p13_guild_donation_badge_policies')]:
        if table not in available:
            policy[key] = {'available': False}
            continue
        row = sql(f'SELECT enabled,version,published_at,SHA2(source,256) FROM {table} WHERE id=1;').strip().split('\t')
        if len(row) != 4:
            raise RuntimeError('Donation policy row is missing')
        policy[key] = {'available': True, 'enabled': row[0] == '1',
                       'version': row[1], 'publishedAt': int(row[2]), 'sourceHash': row[3]}
    profile = sql('SELECT COALESCE(JSON_EXTRACT(player,"$.level"),0),'
                  'COALESCE(JSON_EXTRACT(player,"$.coin"),0),'
                  'COALESCE(JSON_EXTRACT(player,"$.diamond"),0),'
                  'COALESCE((SELECT SUM(j.num) FROM JSON_TABLE(profiles.game,"$.items[*]" '
                  'COLUMNS(item_id VARCHAR(64) PATH "$.id",num BIGINT PATH "$.num")) j '
                  'WHERE j.item_id="item_089"),0) FROM profiles WHERE id=' + str(player_id) + ';').strip().split('\t')
    member = sql('SELECT guild_id,role,donate_day,today_coin,today_badge FROM p13_guild_members '
                 'WHERE player_id=' + str(player_id) + ';').strip().split('\t')
    return {'guildGates': {'management': gate[0] == '1', 'economy': gate[1] == '1',
                           'chat': gate[2] == '1'}, 'policies': policy,
            'playerQualification': {'playerId': player_id,
                                    'level': int(profile[0]) if profile and profile[0] else None,
                                    'coin': int(profile[1]) if len(profile) > 1 else None,
                                    'diamond': int(profile[2]) if len(profile) > 2 else None,
                                    'heldBadge089': int(profile[3]) if len(profile) > 3 else None,
                                    'hasGuild': bool(member and member[0]),
                                    'role': int(member[1]) if len(member) > 1 else None,
                                    'lastDonateDay': member[2] if len(member) > 2 else None,
                                    'lastDayDonatedCoin': int(member[3]) if len(member) > 3 else None,
                                    'lastDayDonatedBadge': int(member[4]) if len(member) > 4 else None}}


def qualified_candidate(source_root, validation, expected_sha):
    if source_root.parent != (ROOT / 'build/candidates').resolve() or not source_root.is_dir():
        raise RuntimeError('Activation requires a direct frozen build/candidates directory')
    proof = json.loads(validation.read_text())
    if not proof.get('qualified') or proof.get('binarySHA256') != expected_sha:
        raise RuntimeError('Activation requires the qualified running binary proof')
    if (ROOT / proof.get('candidate', '')).resolve() != source_root:
        raise RuntimeError('Qualification source differs from selected candidate')
    manifest = json.loads((source_root / 'manifest.json').read_text())
    files = {row['path']: row['sha256'] for row in manifest['files']}
    selected = rules(source_root)
    for rule in selected.values():
        if files.get(rule['constPath']) != rule['constSHA256']:
            raise RuntimeError('Frozen donation rule differs from its candidate manifest')
    return selected


def change_policies(enabled, selected):
    # The shared guild row lock serializes this operation with every real donation.
    box, badge = selected['boxes'], selected['badges']
    target = 1 if enabled else 0
    gate = '@guild_on=1 AND ' if enabled else ''
    expected = (gate + f'@box_version="{box["version"]}" AND @box_hash="{box["sourceHash"]}" '
                f'AND @badge_version="{badge["version"]}" AND @badge_hash="{badge["sourceHash"]}" '
                'AND @cost=100000 AND @amount=2 AND @daily=15 '
                'AND @box_time>=0 AND @box_time<=@published AND @badge_time>=0 AND @badge_time<=@published')
    query = ('START TRANSACTION; '
             'SET @published=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS UNSIGNED); '
             'SELECT management_enabled AND economy_enabled INTO @guild_on FROM p13_guild_policies WHERE id=1 FOR UPDATE; '
             'SELECT version,SHA2(source,256),published_at INTO @box_version,@box_hash,@box_time FROM p13_guild_donate_box_policies WHERE id=1 FOR UPDATE; '
             'SELECT version,SHA2(source,256),coin_cost,badge_amount,daily_limit,published_at INTO @badge_version,@badge_hash,@cost,@amount,@daily,@badge_time '
             'FROM p13_guild_donation_badge_policies WHERE id=1 FOR UPDATE; '
             f'SET @ready=({expected}); '
             f'UPDATE p13_guild_donate_box_policies SET enabled={target},'
             f'published_at=IF({target}=1 AND published_at=0,@published,published_at) WHERE id=1 AND @ready=1; '
             f'UPDATE p13_guild_donation_badge_policies SET enabled={target},'
             f'published_at=IF({target}=1 AND published_at=0,@published,published_at) WHERE id=1 AND @ready=1; '
             'COMMIT; SELECT COALESCE(@ready,0);')
    if sql(query).strip() != '1':
        raise RuntimeError('Policy preconditions changed; neither local policy was changed')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['inspect', 'activate', 'deactivate'], nargs='?', default='inspect')
    parser.add_argument('--player-id', type=int, default=4)
    parser.add_argument('--source-root', type=Path, default=ROOT)
    parser.add_argument('--validation', type=Path)
    parser.add_argument('--expected-binary-sha')
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--record', type=Path)
    args = parser.parse_args()
    if args.player_id <= 0:
        raise RuntimeError('Use an existing positive player ID')
    source_root = args.source_root.resolve()
    live = operations().get('binarySha256')
    before = inspect(args.player_id)
    result = {'action': args.action, 'applied': False, 'binarySHA256': live, 'before': before,
              'rule': 'paid coin donation -> P1 actual claim -> original badge donation -> daily box',
              'originalGatesNotChanged': True, 'profileInventoryNotChangedByTool': True}
    post_error = None
    record_written = False
    if args.action != 'inspect':
        if not args.validation or not re.fullmatch('[0-9a-f]{64}', args.expected_binary_sha or ''):
            raise RuntimeError('Policy actions require --validation and exact --expected-binary-sha')
        if live != args.expected_binary_sha:
            raise RuntimeError('Running binary differs from requested activation target')
        selected = qualified_candidate(source_root, args.validation.resolve(), live)
        if not all(value['available'] for value in before['policies'].values()):
            raise RuntimeError('Publish the migrated source first; no table will be created by this tool')
        result['namedRules'] = selected
        if args.apply:
            if not args.record or args.record.exists():
                raise RuntimeError('Applying requires a new --record path; previous records are immutable')
            fingerprints = protected_fingerprints()
            with open('/tmp/hyjs-isolated-mysql.lock', 'a') as lock:
                try:
                    fcntl.flock(lock.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError:
                    raise RuntimeError('SQL window is busy; policy unchanged') from None
                try:
                    change_policies(args.action == 'activate', selected)
                    result['applied'] = True
                    result['after'] = inspect(args.player_id)
                    result['protectedProfilesSame'] = protected_fingerprints() == fingerprints
                    result['runtimeHashSame'] = operations().get('binarySha256') == live
                    if not result['protectedProfilesSame'] or not result['runtimeHashSame']:
                        result['status'] = 'APPLIED_CONCURRENT_CHANGE_REQUIRES_REVIEW'
                        post_error = 'Concurrent profile/runtime change observed; inspect activation record before further work'
                except Exception:
                    result['status'] = 'FAILED_ACTION_INSPECT_POLICY_BEFORE_RETRY'
                    raise
                finally:
                    args.record.parent.mkdir(parents=True, exist_ok=True)
                    with args.record.open('x') as output:
                        output.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
                    record_written = True
    if args.record and not record_written:
        args.record.parent.mkdir(parents=True, exist_ok=True)
        with args.record.open('x') as output:
            output.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(result, ensure_ascii=False))
    if post_error:
        raise RuntimeError(post_error)


if __name__ == '__main__':
    main()
