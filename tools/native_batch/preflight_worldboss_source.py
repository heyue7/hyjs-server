"""Pure type18 source/admission and pending-overlay checks, without engine/device access."""
import argparse
import copy
import importlib.util
import json
from pathlib import Path
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
sys.dont_write_bytecode = True
sys.path[:0] = [str(ROOT), str(ROOT / 'tools'), str(Path(__file__).resolve().parent)]
from server.codec import message
from export_worldboss_source import SCOPE, sha, validate_full_boss


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    module.ROOT = ROOT
    return module


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', type=Path, required=True)
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--applied-overlay', type=Path,
                        help='Verify exact applied tool hashes and exercise host admission after the reviewed merge')
    args = parser.parse_args()
    manifest, report = args.manifest.resolve(), args.report.resolve()
    if not manifest.is_relative_to(ROOT / 'analysis/p6') or report.exists() or not report.is_relative_to(ROOT / 'analysis/p6'):
        raise ValueError('Use owned source and a new pure preflight report under analysis/p6')
    source = json.loads(manifest.read_text())
    if source.get('nativeCompared') or source.get('authoritative') or source.get('engineExecuted'):
        raise ValueError('Input preparation cannot claim completed native evidence')
    pending_root = ROOT / 'analysis/p6/worldboss18-tools-pending-v201'
    overlay_path = pending_root / 'manifest.json'
    overlay = json.loads(overlay_path.read_text())
    applied_path = args.applied_overlay.resolve() if args.applied_overlay else None
    if applied_path and not applied_path.is_relative_to(ROOT / 'analysis/p6'):
        raise ValueError('Use the reviewed project applied-overlay report')
    applied = json.loads(applied_path.read_text()) if applied_path else None
    applied_entries = {entry['path']: entry for entry in applied['files']} if applied else {}
    if applied and applied['pendingManifestSHA256'] != sha(overlay_path.read_bytes()):
        raise ValueError('Applied overlay refers to another pending manifest')
    entries = []
    for entry in overlay['files']:
        current = sha((ROOT / entry['path']).read_bytes())
        pending = sha((ROOT / entry['snapshot']).read_bytes())
        if pending != entry['sha256']:
            raise ValueError('Pending source snapshot changed: ' + entry['path'])
        expected = applied_entries[entry['path']]['appliedSHA256'] if applied else entry['beforeSHA256']
        if current != expected:
            raise ValueError('Shared host source changed; root must rebase the minimal typed18 delta: ' + entry['path'])
        entries.append({'path': entry['path'], 'snapshot': entry['snapshot'],
                        'currentSHA256': current, 'beforeSHA256': entry['beforeSHA256'],
                        'pendingSHA256': pending, 'currentMatchesBefore': current == entry['beforeSHA256'],
                        'currentMatchesApplied': current == expected if applied else None})
    admission_root = ROOT if applied else pending_root
    prepare = load('p12_checked_prepare', admission_root / 'tools/native_batch/prepare.py')
    collector = load('p12_checked_collector', admission_root / 'tools/native_batch/collect.py')
    checks = []
    for case in source['cases']:
        raw = (manifest.parent / case['wire']).read_bytes()
        typed_raw = (manifest.parent / case['typedCallback']).read_bytes()
        if sha(raw) != case['wireSHA256'] or sha(typed_raw) != case['typedCallbackSHA256']:
            raise ValueError('Exact type18 wire/callback identity changed')
        wire, typed = message('SCFightPlayback'), message('SCWorldBossFight')
        wire.ParseFromString(raw)
        typed.ParseFromString(typed_raw)
        validate_full_boss(ROOT, typed, case['bossLevel'])
        actor_hashes = [sha(actor.SerializeToString(deterministic=True)) for actor in typed.battlePlayer.formation]
        boss_hashes = [sha(actor.SerializeToString(deterministic=True)) for actor in typed.battleMonsterList[0].formation]
        if actor_hashes != case['sourceActorSHA256'] or boss_hashes != case['sourceBossMemberSHA256']:
            raise ValueError('Complete source actors changed after manifest binding')
        if source['sourceKind'] == 'finite-actual-p5-original-table-diagnostic':
            if actor_hashes != [source['exactActualP5ActorSHA256']]:
                raise ValueError('Finite actual P5 actor has been numerically edited')
        elif source['sourceKind'] == 'finite-historical-actual-p5-party001003-original-table-diagnostic':
            if actor_hashes != source['exactActualP5ActorSHA256List']:
                raise ValueError('Historical actual complete001003 actors have been edited')
            if sha((ROOT / source['actorSource']).read_bytes()) != source['actorSourceSHA256']:
                raise ValueError('Historical actual party provenance changed')
            if sha((ROOT / source['profileSource']).read_bytes()) != source['profileSourceSHA256']:
                raise ValueError('Historical actual-profile provenance changed')
        if prepare.load_worldboss_typed_source(case, manifest, 'highest') != typed_raw:
            raise AssertionError('Actual prepare admission did not retain exact SC callback bytes')
        staged = dict(case, batchTypedCallback='000.worldboss-start.bin')
        collector.validate_typed_preflight(staged, wire, 'highest', '000.pb')
        collector.validate_typed_execution(staged, {'typedCallbackSHA256': case['typedCallbackSHA256']})
        checks.append({'case': case['case'], 'fourOriginalMembersAndAllSourceBytesBound': True,
                       'prepareAndCollectorPureAdmission': True})
    # Negative examples stay disposable and cannot become a source or native golden.
    baseline = source['cases'][0]
    typed = message('SCWorldBossFight')
    typed.ParseFromString((manifest.parent / baseline['typedCallback']).read_bytes())
    rejected = []
    for change in ('drop-body', 'change-hp', 'drop-original-skills', 'multiply-blood', 'empty-scene'):
        changed = message('SCWorldBossFight')
        changed.CopyFrom(typed)
        if change == 'drop-body':
            del changed.battleMonsterList[0].formation[-1]
        elif change == 'change-hp':
            changed.battleMonsterList[0].formation[0].prop.hp += 1
        elif change == 'drop-original-skills':
            changed.battleMonsterList[0].formation[0].skillOrder = ''
        elif change == 'multiply-blood':
            changed.battleMonsterList[0].formation[0].bloodNumber = 4
        else:
            changed.fightScene = ''
        try:
            validate_full_boss(ROOT, changed, baseline['bossLevel'])
        except ValueError:
            rejected.append(change)
        else:
            raise AssertionError('Malformed diagnostic accepted: ' + change)
    with tempfile.TemporaryDirectory(prefix='hyjs-worldboss18-pure-') as temp:
        directory = Path(temp)
        local_case = copy.deepcopy(baseline)
        for key in ('wire', 'typedCallback'):
            (directory / local_case[key]).write_bytes((manifest.parent / local_case[key]).read_bytes())
        changed = message('SCFightPlayback')
        changed.ParseFromString((directory / local_case['wire']).read_bytes())
        changed.randSeed += 1
        (directory / local_case['wire']).write_bytes(changed.SerializeToString(deterministic=True))
        try:
            prepare.load_worldboss_typed_source(local_case, directory / 'manifest.json', 'highest')
        except RuntimeError:
            rejected.append('seed-mismatch')
        else:
            raise AssertionError('Typed wrapper seed mismatch accepted')
    result = {'status': 'PASS_PURE_PREPARATION_ONLY', 'sourceManifest': str(manifest),
              'sourceManifestSHA256': sha(manifest.read_bytes()), 'sourceKind': source['sourceKind'],
              'caseChecks': checks, 'negativeChecks': rejected,
              'overlayManifest': str(overlay_path.relative_to(ROOT)),
              'overlayManifestSHA256': sha(overlay_path.read_bytes()), 'overlayFiles': entries,
              'admissionSource': 'applied-host' if applied else 'pending-snapshot',
              'appliedOverlayReport': str(applied_path.relative_to(ROOT)) if applied_path else None,
              'previousPrivateCallerCompileProof': str((pending_root / 'csharp-build-proof.json').relative_to(ROOT)),
              'hostSharedFilesChangedByThisTask': bool(applied), 'deviceInspected': False,
              'engineExecuted': False, 'nativeCompared': False, 'globalAuthoritative': False,
              'nextStep': ('Root private caller update, prepare highest, original Kit start once, collect, independent comparison.'
                           if applied else 'Root finishes current pinned P7 capture before applying exact beforeSHA overlay; '
                           'then private caller update, prepare highest, original Kit start once, collect, independent comparison.')}
    report.parent.mkdir(parents=True, exist_ok=True)
    report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'report': str(report), 'cases': len(checks), 'negativeChecks': len(rejected),
                      'hostOverlayFilesReady': len(entries), 'engineExecuted': False}))


if __name__ == '__main__':
    main()
