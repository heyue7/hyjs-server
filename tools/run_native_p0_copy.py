"""Replay the observed 1-2 touch route; read actual receipts, never submit battle results."""
import argparse
import json
from pathlib import Path
import re
import time

from device_demo import Device, ROOT
from demo_accounts import profile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--serial', required=True)
    parser.add_argument('--player-id', type=int, default=3)
    parser.add_argument('--label', required=True)
    args = parser.parse_args()
    if not re.fullmatch(r'[a-z0-9-]{1,60}', args.label):
        raise RuntimeError('Use a short evidence label')
    device = Device(args.serial)

    def state():
        _, player, game = profile(args.player_id)
        run = game.get('run') or {}
        if run.get('mission') != '2' or run.get('chapter') != '1':
            raise RuntimeError('Expected the already entered 1-2 map')
        return player, game, run, run.get('fight') or {}

    def guard():
        device.require_game_surface_accessible()
        if not device.game_is_foreground():
            raise RuntimeError('Game left foreground; stop the same route')

    def move(x, y, duration):
        guard()
        device.run('shell', 'input', 'swipe', 1400, 850, x, y, duration)
        time.sleep(1)

    def fight(task):
        player, game, run, current = state()
        if current.get('task') != task:
            raise RuntimeError('Unexpected task; inspect current map before another move')
        print(json.dumps({'observed_native_task': task}), flush=True)
        deadline = time.monotonic() + 85
        while not current.get('settled'):
            guard()
            if time.monotonic() >= deadline:
                print(device.capture(ROOT / f'analysis/device/{args.label}-pending-{task}.png'), flush=True)
                raise RuntimeError('Same battle still pending; observe it before any retry')
            time.sleep(5)
            player, game, run, current = state()
        if (current.get('result') or {}).get('fightResult') != 1:
            raise RuntimeError('Native battle lost; stop and inspect growth options')
        evidence = {'player': player, 'run': run, 'equipment': game.get('equipment', []),
                    'formation': game['formation'], 'source': 'native touch route and real receipt'}
        (ROOT / f'analysis/p0/{args.label}-task-{task}.json').write_text(
            json.dumps(evidence, ensure_ascii=False, indent=2))
        print(json.dumps({'native_win': task, 'coin': player['coin'], 'level': player['level'],
                          'passed': run['passed']}), flush=True)
        time.sleep(3)

    def dismiss():
        guard()
        device.run('shell', 'input', 'tap', 1800, 900)
        time.sleep(3)

    # The caller enters the map and verifies the native auto-battle setting first.
    _, _, run, current = state()
    if run.get('passed') or current:
        raise RuntimeError('Start from the fresh observed 1-2 map; do not restart a pending battle')
    move(1900, 220, 3500)
    fight('5')
    dismiss()
    for x, y, duration in [(900, 240, 4000), (450, 650, 2500), (1400, 200, 3500)]:
        _, _, _, current = state()
        if current.get('task') == '7' and not current.get('settled'):
            break
        move(x, y, duration)
    fight('7')
    dismiss()
    move(600, 330, 3500)
    move(2200, 250, 3500)
    fight('8')
    print(device.capture(ROOT / f'analysis/device/{args.label}-victory.png'), flush=True)


if __name__ == '__main__':
    try:
        main()
    except RuntimeError as error:
        print(json.dumps({'error': str(error)}, ensure_ascii=False), flush=True)
        raise SystemExit(1)
