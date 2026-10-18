"""Capture or verify original guild UI actions using operator-observed coordinates.

Only explicit tap/relogin commands operate the reserved Android device. No game
action is sent through an HTTP shortcut and no SQL writes prepare rewards. The
coordinator supplies coordinates from a current original-resolution screenshot.
"""
import argparse
import hashlib
import json
from pathlib import Path
import sys
import time

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
from tools.device_demo import Device
from tools.p13_guild_donation_ops import inspect
from tools.shared_mysql import sql
from tools.release_go import operations


def snapshot(player_id):
    result = inspect(player_id)
    tables = set(sql('SELECT TABLE_NAME FROM information_schema.tables WHERE table_schema=DATABASE() '
                     'AND TABLE_NAME IN ("p13_guild_donation_badge_sources",'
                     '"p13_guild_donate_box_sources","p13_guild_donate_box_claims");').split())
    pid = int(player_id)
    result['boxScore'] = int(sql('SELECT COALESCE(SUM(score),0) FROM p13_guild_donate_box_sources '
                                 f'WHERE guild_id=(SELECT guild_id FROM p13_guild_members WHERE player_id={pid}) '
                                 'AND day=DATE_FORMAT(CONVERT_TZ(UTC_TIMESTAMP(),"+00:00","+08:00"),"%Y-%m-%d");').strip())
    result['boxClaims'] = int(sql('SELECT COUNT(*) FROM p13_guild_donate_box_claims '
                                  f'WHERE player_id={pid} AND day=DATE_FORMAT(CONVERT_TZ(UTC_TIMESTAMP(),'
                                  '"+00:00","+08:00"),"%Y-%m-%d");').strip())
    result['paidBadgeSources'] = 0
    result['paidBadgeDeliveriesClaimed'] = 0
    if 'p13_guild_donation_badge_sources' in tables:
        row = sql('SELECT COUNT(*),COALESCE(SUM(d.claimed_at>0),0) FROM '
                  'p13_guild_donation_badge_sources s JOIN p1_reward_deliveries d ON d.id=s.delivery_id '
                  f'AND d.player_id=s.player_id WHERE s.player_id={pid} AND '
                  's.day=DATE_FORMAT(CONVERT_TZ(UTC_TIMESTAMP(),"+00:00","+08:00"),"%Y-%m-%d");').strip().split('\t')
        result['paidBadgeSources'], result['paidBadgeDeliveriesClaimed'] = map(int, row)
    return result


def verify(action, before, after, tier):
    left, right = before['playerQualification'], after['playerQualification']
    if action == 'coin':
        return (right['coin'] == left['coin'] - 100000 and
                after['boxScore'] == before['boxScore'] + 1 and
                after['paidBadgeSources'] == before['paidBadgeSources'] + 1)
    if action == 'badge':
        return (right['heldBadge089'] == left['heldBadge089'] - 1 and
                after['boxScore'] == before['boxScore'] + 8 and
                after['paidBadgeSources'] == before['paidBadgeSources'])
    if action == 'claim':
        reward = [1000, 2000, 3000, 5000][tier - 1]
        return (right['coin'] == left['coin'] + reward and
                after['boxClaims'] == before['boxClaims'] + 1)
    if action == 'mail':
        claimed = after['paidBadgeDeliveriesClaimed'] - before['paidBadgeDeliveriesClaimed']
        return claimed > 0 and right['heldBadge089'] == left['heldBadge089'] + 2 * claimed
    if action == 'relogin':
        return before == after
    return True


def check_count(action, count):
    if not 1 <= count <= 30 or (count != 1 and action not in ('coin', 'badge')):
        raise RuntimeError('--count must be 1..30; only coin/badge support repeated actions')


def check_qualification(action, facts, tier):
    qualification = facts['playerQualification']
    if action == 'coin' and (qualification['coin'] < 100000 or not facts['policies']['badges'].get('enabled')):
        raise RuntimeError('Coin balance or published natural-badge source is unavailable')
    if action == 'badge' and qualification['heldBadge089'] < 1:
        raise RuntimeError('No actual held donation badge; complete P1 reward claim first')
    if action == 'claim' and facts['boxScore'] < [250, 400, 500, 600][tier - 1]:
        raise RuntimeError('Original box score threshold is not reached; no tap or quota edit performed')


def recorded_facts(facts):
    # Keep the observed numbers beside a canonical hash of the complete read.
    return {'playerQualification': facts['playerQualification'],
            **{key: facts[key] for key in ('boxScore', 'boxClaims', 'paidBadgeSources',
                                           'paidBadgeDeliveriesClaimed')},
            'snapshotSHA256': hashlib.sha256(json.dumps(facts, sort_keys=True, separators=(',', ':'),
                                                       ensure_ascii=False).encode()).hexdigest()}


def check_donation_progress(action, before, after):
    if action not in ('coin', 'badge'):
        return
    left, right = before['playerQualification'], after['playerQualification']
    # A pending read may span a commit. Exceeding one action cannot be repaired by another tap.
    limits = [('boxScore', after['boxScore'] - before['boxScore'], 1 if action == 'coin' else 8),
              ('paidBadgeSources', after['paidBadgeSources'] - before['paidBadgeSources'],
               1 if action == 'coin' else 0),
              ('coin spent', left['coin'] - right['coin'], 100000 if action == 'coin' else 0),
              ('badge spent', left['heldBadge089'] - right['heldBadge089'], 1 if action == 'badge' else 0)]
    if any(delta < 0 or delta > maximum for _, delta, maximum in limits):
        raise RuntimeError('Observed donation exceeds or contradicts exactly one action; stopped without retry')
    if (before['guildGates'] != after['guildGates'] or before['policies'] != after['policies'] or
            left['playerId'] != right['playerId'] or left['hasGuild'] != right['hasGuild'] or
            left['role'] != right['role'] or left['diamond'] != right['diamond'] or
            before['boxClaims'] != after['boxClaims'] or
            before['paidBadgeDeliveriesClaimed'] != after['paidBadgeDeliveriesClaimed']):
        raise RuntimeError('Donation context changed during the step; stopped without retry')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['observe', 'navigate', 'coin', 'badge', 'mail', 'claim', 'relogin'])
    parser.add_argument('--serial', required=True, help='Phone reserved by the coordinator for this command')
    parser.add_argument('--player-id', type=int, default=4)
    parser.add_argument('--x', type=int)
    parser.add_argument('--y', type=int)
    parser.add_argument('--tier', type=int, choices=[1, 2, 3, 4], default=1)
    parser.add_argument('--count', type=int, default=1, help='1..30 original UI taps; repetition only for coin/badge')
    parser.add_argument('--account', type=Path, help='Existing saved demo credential file; never printed')
    parser.add_argument('--record', type=Path, required=True)
    parser.add_argument('--expected-binary-sha', required=True)
    args = parser.parse_args()
    check_count(args.action, args.count)
    if args.player_id <= 0 or args.record.exists():
        raise RuntimeError('Use an existing player and a new immutable evidence path')
    if operations().get('binarySha256') != args.expected_binary_sha:
        raise RuntimeError('Running service differs from the reserved phone baseline')
    if args.action not in ('observe', 'relogin') and (args.x is None or args.y is None or args.x < 0 or args.y < 0):
        raise RuntimeError('Supply actual coordinates from the current original-resolution capture')
    if args.action == 'relogin':
        if not args.account or args.account.resolve().parent != ROOT / '.local-env':
            raise RuntimeError('Relogin requires the reserved saved local demo account file')
        saved = json.loads(args.account.read_text())
        if saved.get('player_id') != args.player_id or saved.get('role') not in ('starter', 'systems'):
            raise RuntimeError('Saved demo role/player does not match the reserved account')
    device = Device(args.serial)
    device.require_game_surface_accessible()
    if not device.game_is_foreground():
        raise RuntimeError('Bring the reserved local game to foreground before taking this action')
    args.record.parent.mkdir(parents=True, exist_ok=True)
    before = snapshot(args.player_id)
    check_qualification(args.action, before, args.tier)
    result = {'action': args.action, 'status': 'PENDING', 'before': before,
              'requestedCount': args.count, 'completedCount': 0, 'steps': [],
              'binarySHA256': args.expected_binary_sha,
              'runnerSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'businessActionsViaOriginalUIOnly': True, 'sqlWrites': False}
    result['beforeCapture'] = device.capture(args.record.with_name(args.record.stem + '-before.png'))
    try:
        if args.action not in ('observe', 'relogin'):
            # Read dimensions from the sole initial capture; never capture per donation.
            from PIL import Image
            with Image.open(result['beforeCapture']['evidence']) as frame:
                width, height = frame.size
            if args.x >= width or args.y >= height:
                raise RuntimeError('Observed coordinates exceed the current screenshot size')
        step_before = before
        for index in range(1, args.count + 1):
            step = {'index': index, 'status': 'PENDING', 'before': recorded_facts(step_before),
                    'tapAttempted': False, 'tapSent': False, 'startedAtUnixMs': int(time.time() * 1000)}
            result['steps'].append(step)
            check_qualification(args.action, step_before, args.tier)
            step['binarySHA256'] = operations().get('binarySha256')
            if step['binarySHA256'] != args.expected_binary_sha:
                raise RuntimeError('Running service changed during the batch; no next tap performed')
            if not device.game_is_foreground():
                raise RuntimeError('Game left foreground; no next tap performed')
            if args.action == 'relogin':
                device.restart()
                device.wait_rendered(25)
                device.login_saved(args.account)
            elif args.action != 'observe':
                # The only game action in an iteration is this single observed-coordinate tap.
                step['tapAttempted'] = True
                device.run('shell', 'input', 'tap', args.x, args.y)
                step['tapSent'] = True
            deadline = time.monotonic() + 8
            while True:
                after = snapshot(args.player_id)
                result['after'] = after
                step['after'] = recorded_facts(after)
                check_donation_progress(args.action, step_before, after)
                if verify(args.action, step_before, after, args.tier):
                    break
                if time.monotonic() >= deadline:
                    raise RuntimeError('UI action has not produced the expected server fact; stopped without automatic retry')
                time.sleep(0.15)
            step['status'] = 'PASS_ONE_ORIGINAL_UI_STEP'
            step['finishedAtUnixMs'] = int(time.time() * 1000)
            result['completedCount'] = index
            # The last verified server read is the next step's baseline.
            step_before = after
        result['status'] = ('OBSERVED' if args.action in ('observe', 'navigate') else
                            'PASS_ONE_ORIGINAL_UI_STEP' if args.count == 1 else 'PASS_ORIGINAL_UI_BATCH')
    except Exception as error:
        result['status'] = 'FAIL_STEP_PRESERVE_PARTIAL_EVIDENCE'
        result['failure'] = str(error)[:1000]
        if result['steps'] and result['steps'][-1]['status'] == 'PENDING':
            result['steps'][-1]['status'] = 'FAIL_STOP_WITHOUT_RETRY'
            result['steps'][-1]['failure'] = result['failure']
        raise
    finally:
        try:
            result['afterCapture'] = device.capture(args.record.with_name(args.record.stem + '-after.png'))
        except Exception:
            result['afterCaptureUnavailable'] = True
        with args.record.open('x') as output:
            output.write(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': result['status'], 'record': str(args.record)}, ensure_ascii=False))


if __name__ == '__main__':
    main()
