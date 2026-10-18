"""Stage SHA-verified original battle inputs for the device's private batch runner."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from device_demo import Device, PACKAGE


def load_rush_typed_source(case, manifest, selector):
    """Validate actual typed source and diagnostic transport before any device IO."""
    if not case.get('typedCallback'):
        return None
    original = manifest.parent / case['typedCallback']
    recorded = selector == 'highest' and case.get('nativeScope') in ('original-pve-playback-ai', 'original-pve-manual-then-auto-ai')
    if selector not in ('pve-auto', 'highest') or (selector == 'highest' and not recorded) or case.get('battleType') not in (9, 10) or original.parent != manifest.parent or not original.name.endswith('.SCRushFightStart.pb'):
        raise RuntimeError('Use an exact named automatic or recorded Rush typed source')
    raw = original.read_bytes()
    if hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256'):
        raise RuntimeError('Original Rush typed source SHA mismatch')
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    typed = message('SCRushFightStart'); typed.ParseFromString(raw)
    wire = message('SCFightPlayback'); wire.ParseFromString((manifest.parent / case['wire']).read_bytes())
    # Original BattleFighterData dereferences both Inst messages without a
    # null guard. Go default getters cannot make a missing source message valid.
    if (not typed.HasField("battlePlayer") or not typed.battlePlayer.HasField("inst")
            or any(not monster.HasField("inst") for monster in typed.battleMonsterList)):
        raise RuntimeError("Original Rush typed source requires player and monster Inst messages")
    for fighter in [typed.battlePlayer, *typed.battleMonsterList]:
        if any(not actor.HasField("prop") or not actor.HasField("inst") for actor in fighter.formation):
            raise RuntimeError("Original Rush typed source requires each actor prop and Inst messages")
    scheduled = (case.get('nativeScope') == 'original-pve-manual-then-auto-ai'
                 and case.get('manualControlSource') == 'prepublished-diagnostic-schedule-v1')
    expected_type = {1: 9, 2: 10}.get(typed.rushType)
    if (expected_type != wire.battleType or expected_type != case['battleType']
            or typed.fightId != wire.fightid or typed.randSeed != wire.randSeed
            or len(typed.battleMonsterList) != 1 or typed.winKey != 0 or typed.winValue != 0
            or wire.HasField('defender1') or wire.HasField('optRecord') != (recorded and not scheduled) or wire.winConditions
            or wire.attackerFactor != 1 or wire.defenderFactor != 1
            or wire.maxRound != (typed.maxRound if typed.maxRound > 0 else 40)
            or typed.battlePlayer != wire.attacker or typed.battleMonsterList[0] != wire.defender2):
        raise RuntimeError('Original typed Rush source and diagnostic projection differ')
    if scheduled:
        delay = case.get('manualAutoStartRoundTime')
        if (not isinstance(delay, (int, float)) or not math.isfinite(delay) or delay < 0 or delay > 110
                or wire.HasField('optRecord')
                or any(case.get(key) for key in ('originalEnd', 'originalEndSHA256', 'originalRecordSHA256'))):
            raise RuntimeError('Explicit manual diagnostic schedule cannot carry an observed End or supplied playback record')
    if recorded and not scheduled:
        end_path = manifest.parent / case.get('originalEnd', '')
        if end_path.parent != manifest.parent or not end_path.name.endswith('.CSRushFightEnd.pb'):
            raise RuntimeError('Recorded Rush requires its exact original End source')
        end_raw = end_path.read_bytes()
        if hashlib.sha256(end_raw).hexdigest() != case.get('originalEndSHA256'):
            raise RuntimeError('Original actual Rush End source SHA mismatch')
        end = message('CSRushFightEnd'); end.ParseFromString(end_raw)
        record_raw = wire.optRecord.SerializeToString(deterministic=True)
        if (not end.HasField('optRecord') or end.optRecord != wire.optRecord
                or end.fightId != typed.fightId
                or hashlib.sha256(record_raw).hexdigest() != case.get('originalRecordSHA256')):
            raise RuntimeError('Actual original Rush record identity or bytes differ')
        if case.get('nativeScope') == 'original-pve-manual-then-auto-ai':
            player_ops = [op for op in end.optRecord.skillOpts if op.camp == 0]
            delay = case.get('manualAutoStartRoundTime')
            if (not isinstance(delay, (int, float)) or not math.isfinite(delay) or delay < 0 or delay > 110
                    or not player_ops or player_ops[0].roundNum != 1 or player_ops[0].roundTime != delay):
                raise RuntimeError('Manual-to-auto original observed control clock differs')
    return raw


def load_worldboss_typed_source(case, manifest, selector):
    """Bind full original type18 callback; initial independent probe uses the nonempty-scene branch."""
    source = manifest.parent / case.get('typedCallback', '')
    if (selector != 'highest' or case.get('battleType') != 18
            or case.get('nativeScope') != 'original-worldboss-automatic-ai'
            or source.parent != manifest.parent or not source.name.endswith('.SCWorldBossFight.pb')):
        raise RuntimeError('Independent WorldBoss18 requires exact highest typed source')
    raw = source.read_bytes()
    if hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256'):
        raise RuntimeError('Original WorldBoss typed source SHA changed')
    sys.path.insert(0,str(ROOT))
    from server.codec import message
    typed=message('SCWorldBossFight');typed.ParseFromString(raw)
    wire=message('SCFightPlayback');wire.ParseFromString((manifest.parent/case['wire']).read_bytes())
    if (not typed.HasField('battlePlayer') or not typed.battlePlayer.HasField('inst')
            or not typed.battleMonsterList or not typed.battleMonsterList[0].HasField('inst')
            or not typed.fightScene or wire.battleType != 18 or wire.fightid != typed.fightId
            or wire.randSeed != typed.randSeed or wire.maxRound != (typed.maxround if typed.maxround>0 else 40)
            or wire.attacker != typed.battlePlayer or wire.defender2 != typed.battleMonsterList[0]
            or wire.HasField('defender1') or wire.HasField('optRecord') or wire.winConditions
            or wire.attackerFactor != 1 or wire.defenderFactor != 1
            or any(not actor.HasField('prop') or not actor.HasField('inst')
                   for fighter in (typed.battlePlayer,typed.battleMonsterList[0]) for actor in fighter.formation)):
        raise RuntimeError('WorldBoss18 original typed projection differs or needs unbound empty-scene context')
    return raw


def load_awake_field_typed_source(case, manifest, selector):
    """Bind independently typed16/5 inputs before selecting a device."""
    specs = {16: ('SCAwakeCopyFightBegin', 'original-awake16-automatic-ai'),
             5: ('SidePvpFightData', 'original-field-pvp5-field002-automatic-ai')}
    name, scope = specs[case['battleType']]
    source = manifest.parent / case.get('typedCallback', '')
    wire_path = manifest.parent / case['wire']
    if (selector != 'highest' or case.get('nativeScope') != scope
            or source.parent != manifest.parent or not source.name.endswith('.' + name + '.pb')
            or wire_path.parent != manifest.parent):
        raise RuntimeError('Awake16/Field5 requires its independent highest typed source')
    raw, wire_raw = source.read_bytes(), wire_path.read_bytes()
    if (hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256')
            or hashlib.sha256(wire_raw).hexdigest() != case.get('wireSHA256')):
        raise RuntimeError('Independent typed input SHA changed')
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    typed = message(name); typed.ParseFromString(raw)
    wire = message('SCFightPlayback'); wire.ParseFromString(wire_raw)
    if case['battleType'] == 16:
        attacker, defender = typed.battlePlayer, typed.battleMonser
        wrong = (not typed.HasField('battlePlayer') or not typed.HasField('battleMonser')
                 or not typed.roomId or wire.HasField('defender1') or not wire.HasField('defender2')
                 or wire.defender2 != defender or wire.fightid != typed.fightId
                 or wire.randSeed != typed.randomSeed
                 or wire.maxRound != (typed.maxRoundNum if typed.maxRoundNum > 0 else 40))
    else:
        attacker, defender = typed.attPlayer, typed.defPlayer
        wrong = (not typed.HasField('attPlayer') or not typed.HasField('defPlayer')
                 or case.get('fieldContext') != 'field_002'
                 or not wire.HasField('defender1') or wire.HasField('defender2')
                 or wire.defender1 != defender or wire.fightid != typed.id
                 or wire.randSeed != typed.randSeed or typed.winPlayerId != 0
                 or typed.attGet or typed.defLose or typed.attFirstPlayerId != 0
                 or wire.maxRound != (typed.maxRoundCount if typed.maxRoundCount > 0 else 40))
    if (wrong or wire.battleType != case['battleType'] or wire.attacker != attacker
            or not attacker.HasField('inst') or not defender.HasField('inst')
            or wire.HasField('optRecord') or wire.winConditions
            or wire.attackerFactor != 1 or wire.defenderFactor != 1
            or any(not actor.HasField('prop') or not actor.HasField('inst')
                   for fighter in (attacker, defender) for actor in fighter.formation)):
        raise RuntimeError('Awake16/Field5 original typed projection differs or supplies a result')
    return raw


def load_wanted_typed_source(case, manifest, selector):
    """Bind the first original single-wave Wanted instance, including damage factor."""
    source = manifest.parent / case.get('typedCallback', '')
    wire_path = manifest.parent / case['wire']
    if (selector != 'highest' or case.get('battleType') != 8
            or case.get('nativeScope') != 'original-wanted-automatic-ai'
            or source.parent != manifest.parent or not source.name.endswith('.SCRebelFightStart.pb')
            or wire_path.parent != manifest.parent):
        raise RuntimeError('Wanted8 requires its independent highest typed source')
    raw, wire_raw = source.read_bytes(), wire_path.read_bytes()
    if (hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256')
            or hashlib.sha256(wire_raw).hexdigest() != case.get('wireSHA256')):
        raise RuntimeError('Wanted8 typed source SHA changed')
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    typed = message('SCRebelFightStart'); typed.ParseFromString(raw)
    wire = message('SCFightPlayback'); wire.ParseFromString(wire_raw)
    if (not typed.HasField('battlePlayer') or not typed.battlePlayer.HasField('inst')
            or len(typed.battleMonsterList) != 1 or not typed.battleMonsterList[0].HasField('inst')
            or wire.battleType != 8 or wire.maxRound != 20 or wire.fightid != typed.fightId
            or wire.randSeed != typed.randSeed or wire.attacker != typed.battlePlayer
            or wire.defender2 != typed.battleMonsterList[0]
            or wire.HasField('defender1') or wire.HasField('optRecord') or wire.winConditions
            or typed.damageFactor not in (1, 2.5) or wire.attackerFactor != typed.damageFactor
            or wire.defenderFactor != 1 or any(not actor.HasField('prop') or not actor.HasField('inst')
               for fighter in (typed.battlePlayer, typed.battleMonsterList[0]) for actor in fighter.formation)):
        raise RuntimeError('Wanted8 original fixed20/single-wave/source/factor projection differs')
    return raw


def load_arena_typed_source(case, manifest, selector):
    """Bind the original arena callback sidecar before touching the device."""
    if not case.get('typedCallback'):
        return None
    original = manifest.parent / case['typedCallback']
    if (selector != 'highest' or case.get('battleType') != 2 or original.parent != manifest.parent
            or not original.name.endswith('.SCPvpFightResult.pb')):
        raise RuntimeError('Use the exact original highest arena typed source')
    raw = original.read_bytes()
    if hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256'):
        raise RuntimeError('Original arena typed source SHA mismatch')
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    typed = message('SCPvpFightResult'); typed.ParseFromString(raw)
    wire = message('SCFightPlayback'); wire.ParseFromString((manifest.parent / case['wire']).read_bytes())
    if (len(typed.playerList) != 2 or any(not player.HasField('inst') for player in typed.playerList)
            or any(not actor.HasField('prop') or not actor.HasField('inst') for player in typed.playerList for actor in player.formation)):
        raise RuntimeError('Original arena callback requires both player Inst and every actor prop/Inst')
    if (wire.battleType != 2 or typed.HasField('winPlayerId') or typed.HasField('result') or typed.boxList
            or typed.fightId != wire.fightid or typed.randSeed != wire.randSeed
            or wire.maxRound != (typed.maxRoundNum if typed.maxRoundNum > 0 else 40)
            or wire.HasField('defender2') or wire.HasField('optRecord') or wire.winConditions
            or not wire.HasField('defender1') or wire.attackerFactor != 1 or wire.defenderFactor != 1
            or typed.playerList[0] != wire.attacker or typed.playerList[1] != wire.defender1):
        raise RuntimeError('Original arena typed callback and diagnostic projection differ or contain result hints')
    return raw


def load_plunder_typed_source(case, manifest, selector):
    """Bind the original mode3 callback and target parameter before device IO."""
    original = manifest.parent / case.get('typedCallback', '')
    if (selector != 'highest' or case.get('battleType') != 3 or original.parent != manifest.parent
            or not original.name.endswith('.SCFightTreasureStartResult.pb')):
        raise RuntimeError('Use exact original highest plunder typed source')
    raw = original.read_bytes()
    if hashlib.sha256(raw).hexdigest() != case.get('typedCallbackSHA256'):
        raise RuntimeError('Original plunder typed source SHA mismatch')
    sys.path.insert(0, str(ROOT))
    from server.codec import message
    typed = message('SCFightTreasureStartResult'); typed.ParseFromString(raw)
    wire = message('SCFightPlayback'); wire.ParseFromString((manifest.parent / case['wire']).read_bytes())
    if (len(typed.playerList) != 2 or any(not player.HasField('inst') for player in typed.playerList)
            or any(not actor.HasField('prop') or not actor.HasField('inst') for player in typed.playerList for actor in player.formation)):
        raise RuntimeError('Plunder requires both original player and actor instances')
    if (not case.get('targetFragId') or wire.battleType != 3 or wire.maxRound != 20
            or any(typed.HasField(field) for field in ('isWin', 'get', 'isGetFrag', 'reward', 'rewardFake1', 'rewardFake2')) or typed.cost
            or typed.fightId != wire.fightid or typed.randSeed != wire.randSeed
            or wire.HasField('defender2') or wire.HasField('optRecord') or wire.winConditions
            or not wire.HasField('defender1') or wire.attackerFactor != 1 or wire.defenderFactor != 1
            or typed.playerList[0] != wire.attacker or typed.playerList[1] != wire.defender1):
        raise RuntimeError('Original plunder typed source and Max20 projection differ or contain result hints')
    return raw


def load_grab_typed_source(case, manifest, selector):
    """Bind original type19 callback before device IO; no winner/fees fabricated."""
    source=manifest.parent/case.get('typedCallback','')
    if (selector!='highest' or case.get('battleType')!=19 or source.parent!=manifest.parent
            or not source.name.endswith('.SCGrab.pb') or case.get('nativeScope')!='original-pvp-automatic-ai'):
        raise RuntimeError('Use independently named original highest Grab19 typed source')
    raw=source.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=case.get('typedCallbackSHA256'):raise RuntimeError('Original Grab19 typed SHA changed')
    sys.path.insert(0,str(ROOT))
    from server.codec import message
    typed=message('SCGrab');typed.ParseFromString(raw)
    wire=message('SCFightPlayback');wire.ParseFromString((manifest.parent/case['wire']).read_bytes())
    if (not typed.HasField('attPlayer') or not typed.HasField('defPlayer')
            or any(not p.HasField('inst') for p in (typed.attPlayer,typed.defPlayer))
            or any(not a.HasField('prop') or not a.HasField('inst') for p in (typed.attPlayer,typed.defPlayer) for a in p.formation)
            or typed.HasField('winPlayerId') or wire.battleType!=19 or wire.fightid!=typed.fightId or wire.randSeed!=typed.randSeed
            or wire.maxRound!=(typed.maxRoundNum if typed.maxRoundNum>0 else 40)
            or wire.attacker!=typed.attPlayer or wire.defender1!=typed.defPlayer
            or wire.HasField('defender2') or wire.HasField('optRecord') or wire.winConditions
            or wire.attackerFactor!=1 or wire.defenderFactor!=1):
        raise RuntimeError('Original Grab19 typed callback and transport differ or provide preset result')
    return raw


def load_limit_typed_source(case, manifest, selector):
    """Independent original Limit20 wrapper and complete actual frozen players."""
    source=manifest.parent/case.get('typedCallback','')
    if (selector!='highest' or case.get('battleType')!=20 or source.parent!=manifest.parent
            or not source.name.endswith('.SCLimitPvpFightResult.pb') or case.get('nativeScope')!='original-pvp-automatic-ai'):
        raise RuntimeError('Use independently named original highest Limit20 typed source')
    raw=source.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=case.get('typedCallbackSHA256'):raise RuntimeError('Original Limit20 typed SHA changed')
    sys.path.insert(0,str(ROOT))
    from server.codec import message
    typed=message('SCLimitPvpFightResult');typed.ParseFromString(raw)
    wire=message('SCFightPlayback');wire.ParseFromString((manifest.parent/case['wire']).read_bytes())
    if (len(typed.playerList)!=2 or any(not p.HasField('inst') for p in typed.playerList)
            or any(not a.HasField('prop') or not a.HasField('inst') for p in typed.playerList for a in p.formation)
            or any(typed.HasField(f) for f in ('winPlayerId','myrank','intergral'))
            or wire.battleType!=20 or wire.fightid!=typed.fightId or wire.randSeed!=typed.randSeed
            or wire.maxRound!=(typed.maxRoundNum if typed.maxRoundNum>0 else 40)
            or wire.attacker!=typed.playerList[0] or wire.defender1!=typed.playerList[1]
            or wire.HasField('defender2') or wire.HasField('optRecord') or wire.winConditions
            or wire.attackerFactor!=1 or wire.defenderFactor!=1):
        raise RuntimeError('Original Limit20 typed callback and transport differ or provide preset result')
    return raw


def load_kf_typed_source(case, manifest, selector):
    """Bind exact original KF payload and separate explicit local realm identity."""
    source=manifest.parent/case.get('typedCallback','')
    if (selector!='highest' or case.get('battleType')!=17 or source.parent!=manifest.parent
            or not source.name.endswith('.KFReplayData.pb')
            or case.get('nativeScope') not in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1')):
        raise RuntimeError('KF source requires separately named local joint-viewer scope')
    raw=source.read_bytes()
    if hashlib.sha256(raw).hexdigest()!=case.get('typedCallbackSHA256'):raise RuntimeError('KF original PB SHA changed')
    sys.path.insert(0,str(ROOT))
    from server.codec import message
    typed=message('KFReplayData');typed.ParseFromString(raw)
    wire=message('SCFightPlayback');wire.ParseFromString((manifest.parent/case['wire']).read_bytes())
    joint=(case.get('viewerServerID'),case.get('viewerPlayerID'))
    projection=case.get('nativeScope')=='original-kf-numeric-pid-wrapper-only-v1'
    same_realm=case.get('nativeScope')=='local-kf-replay-same-realm-distinct-player-viewer-v1'
    indices=([i for i,p in enumerate(typed.playerList) if p.playerId==joint[1]] if projection else [i for i,p in enumerate(typed.playerList) if (p.serverId,p.playerId)==joint])
    if (len(typed.playerList)!=2 or any(not p.HasField('inst') or p.serverId<=0 for p in typed.playerList)
            or ((typed.playerList[0].serverId!=typed.playerList[1].serverId or any(p.playerId<=0 for p in typed.playerList) or typed.playerList[0].playerId==typed.playerList[1].playerId) if same_realm else typed.playerList[0].serverId==typed.playerList[1].serverId) or (not projection and len(indices)!=1) or (projection and (joint[0]!=0 or len(indices)!=2))
            or any(not a.HasField('prop') or not a.HasField('inst') for p in typed.playerList for a in p.formation)
            or typed.HasField('winPlayerId') or typed.HasField('score')):
        raise RuntimeError('KF requires unique SHA-bound scope-specific realm/player source with no result hints')
    i=indices[0]
    if (wire.battleType!=17 or wire.fightid!=typed.warId or wire.randSeed!=typed.randSeed
            or wire.maxRound!=(typed.maxRoundNum if typed.maxRoundNum>0 else 40)
            or wire.attacker!=typed.playerList[i] or wire.defender1!=typed.playerList[1-i]
            or wire.HasField('defender2') or wire.HasField('optRecord') or wire.winConditions
            or wire.attackerFactor!=1 or wire.defenderFactor!=1):
        raise RuntimeError('KF diagnostic transport changed original actors/seed/wrapper values')
    return raw


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', required=True, type=Path)
    parser.add_argument('--priority', action='store_true')
    parser.add_argument('--pending', action='store_true')
    parser.add_argument('--selector', choices=['highest', 'pve-auto', 'pve-compat'], default='highest')
    parser.add_argument('--serial', default='10AF7Q0KSJ003GK')
    args = parser.parse_args()
    manifest = args.manifest.resolve()
    if not any(manifest.is_relative_to(ROOT / 'analysis' / domain) for domain in ('p5', 'p6', 'p7', 'p9', 'p11', 'p12')):
        raise RuntimeError('Use a verified combat evidence manifest')
    data = json.loads(manifest.read_text())
    cases = data['nativePriorityCases' if args.priority else 'cases']
    if manifest.is_relative_to(ROOT / 'analysis/p11') and any(case.get('battleType') != 3 or not case.get('typedCallback') for case in cases):
        raise RuntimeError('P11 diagnostic staging is limited to typed original plunder3 sources')
    if manifest.is_relative_to(ROOT / 'analysis/p7') and any(case.get('battleType') != 16 for case in cases):
        raise RuntimeError('P7 staging is limited to independently typed Awake16')
    if manifest.is_relative_to(ROOT / 'analysis/p12') and any(case.get('battleType') not in (5, 8) for case in cases):
        raise RuntimeError('P12 staging requires independently typed Field5/Wanted8')
    if args.pending:
        cases = [case for case in cases if case.get('status') == 'awaiting-original-automatic-ai']
    if not cases:
        raise RuntimeError('The selected input set is empty')
    inputs = []
    typed_inputs = []
    for case in cases:
        wire = manifest.parent / case['wire']
        timeline = manifest.parent / case['goTimeline'] if case.get('goTimeline') else None
        if wire.parent != manifest.parent or (timeline is not None and timeline.parent != manifest.parent) or wire.suffix != '.pb':
            raise RuntimeError('Invalid evidence filename')
        if hashlib.sha256(wire.read_bytes()).hexdigest() != case['wireSHA256']:
            raise RuntimeError('Original wire input SHA mismatch')
        if timeline is not None and hashlib.sha256(timeline.read_bytes()).hexdigest() != case['goTimelineSHA256']:
            raise RuntimeError('Go timeline SHA mismatch')
        typed = (load_wanted_typed_source(case, manifest, args.selector) if case.get('battleType') == 8
                 else load_awake_field_typed_source(case, manifest, args.selector) if case.get('battleType') in (5, 16)
                 else load_worldboss_typed_source(case, manifest, args.selector) if case.get('battleType') == 18
                 else load_grab_typed_source(case, manifest, args.selector) if case.get('battleType') == 19
                 else load_arena_typed_source(case, manifest, args.selector) if case.get('battleType') == 2
                 else load_plunder_typed_source(case, manifest, args.selector) if case.get('battleType') == 3
                 else load_limit_typed_source(case, manifest, args.selector) if case.get('battleType') == 20
                 else load_kf_typed_source(case, manifest, args.selector) if case.get('typedCallback','').endswith('.KFReplayData.pb')
                 else load_rush_typed_source(case, manifest, args.selector))
        inputs.append(wire.read_bytes())
        typed_inputs.append(typed)
    device = Device(args.serial)
    remote = 'files/native-batch/' + args.selector
    listing = device.text('shell', 'run-as', PACKAGE, 'ls', 'files/native-batch')
    if args.selector in listing.splitlines():
        if 'status.txt' in device.text('shell', 'run-as', PACKAGE, 'ls', remote).splitlines():
            status = device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/status.txt')
            if status.startswith('running'):
                raise RuntimeError('Preserve the running batch')
    staging = ROOT / 'build/native-batch-input' / args.selector
    staging.mkdir(parents=True, exist_ok=True)
    for old in staging.glob('*.pb'):
        old.unlink()
    for old in list(staging.glob('*.wanted-start.bin')) + list(staging.glob('*.awake-start.bin')) + list(staging.glob('*.field-pvp.bin')) + list(staging.glob('*.worldboss-start.bin')) + list(staging.glob('*.grab-start.bin')) + list(staging.glob('*.limit-result.bin')) + list(staging.glob('*.kf-replay.bin')) + list(staging.glob('*.kf-viewer.txt')) + list(staging.glob('*.rush-start.bin')) + list(staging.glob('*.arena-result.bin')) + list(staging.glob('*.plunder-start.bin')) + list(staging.glob('*.plunder-target.txt')) + list(staging.glob('*.rush-auto-start.txt')) + list(staging.glob('*.rush-auto-policy.txt')):
        old.unlink()
    records = []
    for index, (case, raw) in enumerate(zip(cases, inputs)):
        name = case['wire'] if args.selector in ('pve-auto', 'pve-compat') else '%03d.pb' % index
        if args.selector in ('pve-auto', 'pve-compat') and not (len(name) == 6 and name[:3].isdigit() and name.endswith('.pb')):
            raise RuntimeError('Use numbered original PVE inputs')
        (staging / name).write_bytes(raw)
        record_case = dict(case, batchWire=name)
        typed = typed_inputs[index]
        if typed is not None:
            sidecar = Path(name).stem + ('.wanted-start.bin' if case.get('battleType') == 8 else '.awake-start.bin' if case.get('battleType') == 16 else '.field-pvp.bin' if case.get('battleType') == 5 else '.worldboss-start.bin' if case.get('battleType') == 18 else '.grab-start.bin' if case.get('battleType') == 19 else '.limit-result.bin' if case.get('battleType') == 20 else '.kf-replay.bin' if case.get('typedCallback','').endswith('.KFReplayData.pb') else '.arena-result.bin' if case.get('battleType') == 2 else '.plunder-start.bin' if case.get('battleType') == 3 else '.rush-start.bin')
            (staging / sidecar).write_bytes(typed)
            record_case['batchTypedCallback'] = sidecar
            if case.get('nativeScope') in ('local-kf-replay-joint-realm-player-viewer-v1','original-kf-numeric-pid-wrapper-only-v1','local-kf-replay-same-realm-distinct-player-viewer-v1'):
                driver=Path(name).stem+'.kf-viewer.txt'
                data=('%s|%d|%d'%(case['nativeScope'],case['viewerServerID'],case['viewerPlayerID'])).encode('ascii')
                (staging/driver).write_bytes(data)
                record_case.update(batchTypedDriver=driver,typedDriverSHA256=hashlib.sha256(data).hexdigest())
            if case.get('nativeScope') == 'original-pve-manual-then-auto-ai':
                driver = Path(name).stem + '.rush-auto-start.txt'
                data = str(case['manualAutoStartRoundTime']).encode('ascii')
                (staging / driver).write_bytes(data)
                record_case.update(batchTypedDriver=driver, typedDriverSHA256=hashlib.sha256(data).hexdigest())
                if case.get('manualControlSource') == 'prepublished-diagnostic-schedule-v1':
                    policy = Path(name).stem + '.rush-auto-policy.txt'
                    policy_data = case['manualControlSource'].encode('ascii')
                    (staging / policy).write_bytes(policy_data)
                    record_case.update(batchTypedDriverPolicy=policy, typedDriverPolicySHA256=hashlib.sha256(policy_data).hexdigest())
            if case.get('battleType') == 3:
                target = Path(name).stem + '.plunder-target.txt'
                data = case['targetFragId'].encode('utf-8')
                (staging / target).write_bytes(data)
                record_case.update(batchTypedTarget=target, typedTargetSHA256=hashlib.sha256(data).hexdigest())
        records.append(record_case)
    record = {'sourceManifest': str(manifest), 'selector': args.selector, 'cases': records,
              'scope': 'original-engine inputs; AI mode and native results require separate verification',
              'nativeCompared': False, 'authoritative': False}
    (staging / 'manifest.json').write_text(json.dumps(record, indent=2))
    transfer = '/data/local/tmp/hyjs-native-' + args.selector + '-root'
    device.run('shell', 'mkdir', '-p', transfer)
    device.run('push', str(staging) + '/.', transfer, timeout=60)
    device.run('shell', 'run-as', PACKAGE, 'mkdir', '-p', remote)
    # Explicitly enumerate only our previous numbered protobuf inputs.
    previous = device.text('shell', 'run-as', PACKAGE, 'ls', remote).splitlines()
    # Archive the last terminal marker before staging a new batch. Native result
    # files remain available, and the watcher waits for this execution's marker.
    if 'status.txt' in previous:
        terminal = device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/status.txt')
        if terminal.startswith('running'):
            raise RuntimeError('The original native selector is already running')
        archived = 'status-' + hashlib.sha256(terminal.encode()).hexdigest()[:16] + '.txt'
        device.run('shell', 'run-as', PACKAGE, 'mv', remote + '/status.txt', remote + '/' + archived)
    for name in previous:
        if (len(name) == 6 and name[:3].isdigit() and name.endswith('.pb')) or (len(name) == 18 and name[:3].isdigit() and name.endswith('.rush-start.bin')) or (len(name) == 20 and name[:3].isdigit() and name.endswith('.arena-result.bin')) or (name[:3].isdigit() and name[3:] in ('.wanted-start.bin', '.awake-start.bin', '.field-pvp.bin', '.worldboss-start.bin', '.grab-start.bin', '.limit-result.bin', '.kf-replay.bin', '.kf-viewer.txt', '.plunder-start.bin', '.plunder-target.txt', '.rush-auto-start.txt', '.rush-auto-policy.txt')):
            device.run('shell', 'run-as', PACKAGE, 'rm', remote + '/' + name)
    for case in records:
        name = case['batchWire']
        device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/' + name, remote + '/' + name)
        actual = device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + name).split()[0]
        if actual != case['wireSHA256']:
            raise RuntimeError('Private device input SHA mismatch')
        if case.get('batchTypedCallback'):
            typed = case['batchTypedCallback']
            device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/' + typed, remote + '/' + typed)
            if device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + typed).split()[0] != case['typedCallbackSHA256']:
                raise RuntimeError('Private original typed callback SHA mismatch')
        if case.get('batchTypedDriverPolicy'):
            policy = case['batchTypedDriverPolicy']
            device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/' + policy, remote + '/' + policy)
            if device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + policy).split()[0] != case['typedDriverPolicySHA256']:
                raise RuntimeError('Private original driver policy SHA mismatch')
        if case.get('batchTypedDriver'):
            driver = case['batchTypedDriver']
            device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/' + driver, remote + '/' + driver)
            if device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + driver).split()[0] != case['typedDriverSHA256']:
                raise RuntimeError('Private original manual control source SHA mismatch')
        if case.get('batchTypedTarget'):
            target = case['batchTypedTarget']
            device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/' + target, remote + '/' + target)
            if device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + '/' + target).split()[0] != case['typedTargetSHA256']:
                raise RuntimeError('Private original target source SHA mismatch')
    device.run('shell', 'run-as', PACKAGE, 'cp', transfer + '/manifest.json', remote + '/manifest.json')
    if json.loads(device.text('shell', 'run-as', PACKAGE, 'cat', remote + '/manifest.json')) != record:
        raise RuntimeError('Private device manifest did not survive transfer')
    # The verified private copy now owns the batch. This public staging folder
    # is no longer used by the runner or collector.
    device.run('shell', 'rm', '-rf', transfer)
    print(json.dumps({'prepared': len(records), 'privateSelector': args.selector,
                      'manifest': str(staging / 'manifest.json')}))


if __name__ == '__main__':
    main()
