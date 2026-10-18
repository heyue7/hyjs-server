"""Confirm a newly clicked native Demo order through the explicit local adapter."""
import argparse
import http.client
import json
from pathlib import Path
import time
import uuid

from device_demo import Device, ROOT
from demo_accounts import profile, sql


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--player', type=int, default=4)
    parser.add_argument('--product', required=True)
    parser.add_argument('--cents', type=int, required=True)
    parser.add_argument('--x', type=int, required=True, help='Observed buy-button x with right black bar')
    parser.add_argument('--y', type=int, required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    if args.cents <= 0:
        raise RuntimeError('A positive frozen test price is required')
    _, before, _ = profile(args.player)
    previous = set(sql(f'SELECT id FROM p3_payment_orders WHERE player_id={args.player}').splitlines())
    device = Device(None)
    device.require_game_surface_accessible()
    if not device.game_is_foreground():
        raise RuntimeError('The native demo must be foreground')
    frame = device.frame()
    left = all(frame.getpixel((x, 400)) == (0, 0, 0) for x in (1, 30, 70, 100))
    right = all(frame.getpixel((x, 400)) == (0, 0, 0) for x in (2699, 2730, 2770, 2799))
    if left == right:
        raise RuntimeError('Observe the viewport before buying')
    x = args.x + (104 if left else 0)
    if not (0 <= x < frame.width and 0 <= args.y < frame.height):
        raise RuntimeError('The observed button is outside the screen')
    device.run('shell', 'input', 'tap', x, args.y)
    deadline = time.monotonic() + 12
    order = None
    while time.monotonic() < deadline:
        rows = [line.split('\t') for line in sql(f'SELECT id,product_id,cents,status FROM p3_payment_orders WHERE player_id={args.player} ORDER BY created_at DESC LIMIT 6').splitlines()]
        fresh = [row for row in rows if row[0] not in previous]
        if fresh:
            if len(fresh) != 1 or fresh[0][1:] != [args.product, str(args.cents), 'pending']:
                raise RuntimeError('Unexpected native order; no confirmation sent')
            order = fresh[0]
            break
        time.sleep(.4)
    if order is None:
        print(device.capture(ROOT / 'analysis/device/native-payment-no-order.png'))
        raise RuntimeError('No new native order; inspect UI before another action')
    event = 'native-test-' + uuid.uuid4().hex
    connection = http.client.HTTPConnection('127.0.0.1', 8088, timeout=8)
    body = {'orderId': order[0], 'eventId': event, 'amountCents': args.cents, 'outcome': 'confirmed'}
    try:
        connection.request('POST', '/local-test/payments/confirm', json.dumps(body), {'Content-Type': 'application/json'})
        response = connection.getresponse()
        result = json.loads(response.read())
        if response.status != 200:
            raise RuntimeError('Local test confirmation failed')
    finally:
        connection.close()
    time.sleep(4)
    _, after, _ = profile(args.player)
    fields = ('coin', 'diamond', 'vip', 'chargeDiamond', 'chargeCount', 'monthCardEndTime', 'permanentCardValid')
    evidence = {'native_click': True, 'test_only': True, 'order_id': order[0], 'event_id': event,
                'product': args.product, 'amount_cents': args.cents, 'confirmation': result,
                'before': {key: before.get(key, 0) for key in fields},
                'after': {key: after.get(key, 0) for key in fields},
                'order': sql("SELECT status,credited,source FROM p3_payment_orders WHERE id='" + order[0] + "'").strip()}
    path = Path(args.output).resolve()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(evidence, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(evidence, ensure_ascii=False))
    print(device.capture(ROOT / 'analysis/device' / (path.stem + '.png')))


if __name__ == '__main__':
    main()
