"""Run pending original PVE inputs from an already verified native diagnostic panel."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from device_demo import Device, PACKAGE


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--manifest', required=True, type=Path)
    parser.add_argument('--serial', default='10AF7Q0KSJ003GK')
    parser.add_argument('--timeout', type=int, default=600)
    parser.add_argument('--resume-running', action='store_true')
    parser.add_argument('--single-case', action='store_true')
    args = parser.parse_args()
    manifest = args.manifest.resolve()
    if not manifest.is_relative_to(ROOT / 'analysis/p6'):
        raise RuntimeError('Use a prepared original combat chain')
    device = Device(args.serial)
    device.require_game_surface_accessible()
    deadline = time.monotonic() + args.timeout
    completed = set()
    while time.monotonic() < deadline:
        data = json.loads(manifest.read_text())
        if data.get('status') == 'complete':
            print('Original native source chain complete', flush=True)
            return
        if str(data.get('status','')).startswith(('incomplete','failed')):
            raise RuntimeError('Source chain stopped: '+str(data['status']))
        cases = [case for case in data['cases'] if case.get('status') == 'awaiting-original-automatic-ai']
        if not cases:
            time.sleep(2)
            continue
        case = cases[0]
        name = case['wire']
        if name in completed:
            time.sleep(2)
            continue
        if len(cases) != 1:
            raise RuntimeError('The dependent source chain must publish one battle at a time')
        device.require_game_surface_accessible()
        if not device.game_is_foreground():
            raise RuntimeError('Original diagnostic game is no longer foreground')
        # Re-observe both non-secret inputs before the one UI action. Original
        # TryStart requires this same server/selector pair and never sends NetAPI.
        labels = [row['text'].replace(' ', '') for row in device.ocr()]
        if '60006' not in labels or 'pve-auto' not in labels:
            raise RuntimeError('Native panel inputs changed; inspect before starting')
        remote = 'files/native-batch/pve-auto/'
        status = device.text('shell', 'run-as', PACKAGE, 'cat', remote + 'status.txt')
        resume = args.resume_running and status.startswith(('running', 'complete'))
        if resume:
            staged = json.loads(device.text('shell', 'run-as', PACKAGE, 'cat', remote + 'manifest.json'))
            entries = staged.get('cases', [])
            if len(entries) != 1 or any(entries[0].get(key) != case.get(key) for key in ('fightId', 'seed', 'wireSHA256', 'wire')):
                raise RuntimeError('Running original case does not match the pending checkpoint')
            actual = device.text('shell', 'run-as', PACKAGE, 'sha256sum', remote + name).split()[0]
            if actual != case['wireSHA256']:
                raise RuntimeError('Running original input SHA changed')
            args.resume_running = False
        else:
            prepare = subprocess.run(['rtk', 'proxy', sys.executable, str(ROOT / 'tools/native_batch/prepare.py'),
                '--manifest', str(manifest), '--selector', 'pve-auto', '--pending', '--serial', args.serial],
                capture_output=True, text=True, timeout=90)
            if prepare.returncode:
                raise RuntimeError('Original PVE input preparation failed: ' + prepare.stdout[-500:])
        # The observed native Kit keeps the same viewport anchor while Android
        # moves its black inset between left and right after an IME closes.
        frame = device.frame()
        rows_y = [round(frame.height * value) for value in (.08,.24,.40,.64,.88)]
        dark = lambda x: all(sum(frame.getpixel((x,y))) <= 6 for y in rows_y)
        left, right = 0, frame.width - 1
        while left < frame.width // 10 and dark(left): left += 1
        while right > frame.width * 9 // 10 and dark(right): right -= 1
        if right - left < frame.width * .85:
            raise RuntimeError('Native viewport changed; inspect before starting')
        if not resume:
            device.run('shell','input','tap',round(left + .808 * (right-left+1)),round(.692 * frame.height))
        case_deadline = min(deadline, time.monotonic() + 300)
        while time.monotonic() < case_deadline:
            time.sleep(2)
            status = device.text('shell', 'run-as', PACKAGE, 'cat', 'files/native-batch/pve-auto/status.txt')
            if status.startswith('failed'):
                raise RuntimeError('Original engine caller failed: ' + status[:800])
            if not status.startswith('complete'):
                continue
            raw = device.run('exec-out', 'run-as', PACKAGE, 'cat',
                'files/native-batch/pve-auto/' + case['nativeResult'], timeout=60)
            # The previous complete marker may remain until Unity starts the
            # next coroutine. Wait for the new result, never parse cat errors.
            if not raw.lstrip().startswith(b'{'):
                continue
            try:
                rows = [json.loads(row) for row in raw.splitlines() if row.strip()]
            except json.JSONDecodeError:
                continue
            if not rows or rows[-1].get('kind') != 'result':
                continue
            source = rows[0]
            if source.get('fightId') != case['fightId'] or source.get('seed') != case['seed']:
                # Older completed output survives staging to preserve evidence;
                # wait for this exact new input instead of treating the old marker as completion.
                continue
            if source.get('scope') != 'original-pve-automatic-ai' or rows[-1].get('kind') != 'result':
                raise RuntimeError('Original automatic battle result is incomplete')
            target = manifest.parent / case['nativeResult']
            if target.parent != manifest.parent:
                raise RuntimeError('Invalid original result filename')
            temporary = target.with_suffix('.pending')
            temporary.write_bytes(raw)
            temporary.replace(target)
            completed.add(name)
            print(json.dumps({'wire':name,'bytes':len(raw),'sha256':hashlib.sha256(raw).hexdigest(),
                              'attackerWon':rows[-1]['attackerWon']}), flush=True)
            if args.single_case:
                return
            break
        else:
            raise RuntimeError('Original engine case timeout; retain its inputs and partial output')
    raise RuntimeError('Original native source chain did not finish within the observation window')


if __name__ == '__main__':
    main()
