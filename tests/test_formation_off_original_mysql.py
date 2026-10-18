"""Prebuilt managed original formation-off /20111/cold contract; no compiler.

Only isolated Player20 is a finite slot qualification. The003 card comes from
a real local-wallet30yuan order and the two original box choices. No resource,
card, coin, stage or live profile is injected. Execution is root coordinated.
"""
import concurrent.futures
import fcntl
import json
import os
from pathlib import Path
import secrets
import tempfile
import time
import types
import unittest
import uuid

from google.protobuf.json_format import MessageToDict
from hyjs_managed_mysql import ManagedMySQL
import test_growth_resource_loop_mysql as shared
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table
from server.codec import BY_NAME, CLASSES, parse_http

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'build/candidates/growth-glory-sources-r3-20261017'
QUALIFICATION = ROOT / 'analysis/p3/growth-glory-sources-r3-incremental-20261017.json'
BINARY = ROOT / 'build/hyjs-server-growth-glory-sources-r3-20261017-pending'
BINARY_SHA = 'a546c6b08b0df266e5aeb33a5456d2ab2e17f48ba2a3b9ab2673566401af13c1'


def preflight():
    for key, value in {'HYJS_GROWTH_RESOURCE_SOURCE': SOURCE,
                       'HYJS_GROWTH_RESOURCE_BINARY': BINARY,
                       'HYJS_GROWTH_RESOURCE_VALIDATION': QUALIFICATION}.items():
        os.environ.setdefault(key, str(value))
    source, binary, manifest_sha = shared.preflight()
    if source != SOURCE or shared.sha(binary) != BINARY_SHA:
        raise AssertionError('Use the fixed qualified glory-r3 source and prebuilt app')
    vectors = {'CSFormationList': {},
               'CSFormationOn': {'id': 'hero_003', 'guid': 'actual-owned-guid', 'formatPos': 2},
               'CSFormationOff': {'id': 'hero_003', 'guid': 'actual-owned-guid', 'formatPos': 2},
               'CSGenerateOrder': {'paymentId': 'local-p3-red-card', 'money': 30.0,
                                   'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
               'CSItemUse': {'costItem': {'id': 'item_053', 'num': 1}, 'getItemIndex': 2}}
    for name, fields in vectors.items():
        raw = shared.wire.encode_original_http(name, fields, 2, b'0' * 16, 20111001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original frame contract changed: ' + name)
    if (BY_NAME['CSFormationList'], BY_NAME['SCFormationList']) != (20111, 20112):
        raise AssertionError('Original formation-list opcodes changed')
    choices = {(r['itemId'], r['odds']): r['result'] for r in original_table('ItemUse')}
    if choices.get(('item_023', '5')) != 'item_053,1' or choices.get(('item_053', '2')) != 'hero_003,1':
        raise AssertionError('Original003 acquisition choices changed')
    for relative in ['analysis/tables/TableItemUse.tsv', 'analysis/tables/TablePayment.tsv',
                     'server/codec.py', 'server/protocol/schema.json']:
        if shared.sha(source / relative) != shared.sha(ROOT / relative):
            raise AssertionError('Original runtime contract differs: ' + relative)
    relative = 'hyjs-server/service/formationService.go'
    text = (source / relative).read_text()
    if any(s not in text for s in ['func formationListResult(', 'if formationHero(slot) == nil {',
                                   'func (s *GameService) formationOff(', 'slot.Things = remaining',
                                   '主角不能下阵']):
        raise AssertionError('Qualified empty-slot/original-off integration missing')
    return source, binary, manifest_sha, {'originalRequests': len(vectors), 'requestOpcode': 20111,
        'responseOpcode': 20112, 'formationSourceSHA256': shared.sha(source / relative),
        'qualificationSHA256': shared.sha(QUALIFICATION), 'finitePreparation': 'isolated Player20 only'}


@unittest.skipUnless(os.environ.get('HYJS_FORMATION_OFF_MYSQL_TEST') == '1', 'Root managed prebuilt SQL window')
class FormationOffOriginalMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, wallet_body, wallet = transport.json_request, transport.wallet_body, transport.wallet

    def test_actual_acquire_on_off_20111_empty_slot_cold_and_reform(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contract = preflight()
        self.binary = str(binary)
        output = Path(os.environ.get('HYJS_FORMATION_OFF_OUTPUT',
            ROOT / 'analysis/growth-first-20261016/formation/http-mysql-r1.json')).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/formation'))
        self.assertFalse(output.exists(), 'Preserve prior validation evidence')
        report = dict(status='PENDING', fullChain=False, binarySHA256=BINARY_SHA,
                      sourceManifestSHA256=manifest_sha, runnerSHA256=shared.sha(__file__), contract=contract,
                      phoneUI=False, productionModified=False, externalPayment=False,
                      sourceOfCard='local wallet3000cents -> original30yuan red-card order ->023/5 ->053/2',
                      finitePreparation='isolated Player20 only; no card/item/currency injection', wire=[])
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-formation-off-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('formation_off')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    for table, expected in {'profiles': {'id', 'player', 'game'},
                            'local_wallet_entries': {'player_id', 'kind', 'delta_cents'},
                            'p3_payment_orders': {'player_id', 'status', 'cents', 'credited'}}.items():
                        columns = self.sql('SELECT COLUMN_NAME FROM information_schema.COLUMNS '
                            f"WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='{table}';").split()
                        self.assertTrue(expected <= set(columns), table + ' schema contract')
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-formation-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName': '阵容保护', 'heroId': 'hero_001'}, protected, 20111000, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    identity = 'demo-formation-owner-' + suffix
                    owner = self.login(identity)
                    self.assertGreater(owner.playerId, 1)
                    stamp = 20111001

                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        reply = self.call(name, fields, owner, stamp, expected)
                        report['wire'].append({'request': name, 'timestamp': stamp, 'fields': fields,
                            'response': expected, 'value': MessageToDict(reply, preserving_proto_field_name=True)})
                        return reply

                    def listing(guids):
                        before = self.profile(owner)
                        reply = http('CSFormationList', {}, 'SCFormationList')
                        seen = []
                        for slot in reply.formationList:
                            heroes = [t for t in slot.things if t.pos == 0]
                            self.assertEqual(len(heroes), 1, '20111 must never send an empty UI hero cell')
                            self.assertTrue(heroes[0].id and heroes[0].guid)
                            seen.append(heroes[0].guid)
                        self.assertEqual(seen, guids)
                        self.assertEqual(self.profile(owner), before, '20111 query must retain internal slot and inventory')
                        return reply

                    def reject(fields):
                        before = self.profile(owner)
                        http('CSFormationOff', fields, 'ErrorMessage')
                        self.assertEqual(self.profile(owner), before)

                    http('CSPlayerCreate', {'playerName': '原卸阵真链', 'heroId': 'hero_001'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    main = initial[0]['heroWarId']
                    self.assertEqual((initial[0].get('coin', 0), initial[0].get('diamond', 0)), (0, 0))
                    self.wallet('add', owner, requestKey=str(uuid.uuid4()), amountCents=3000)
                    products = http('CSGetPaymentInfo', {'channelId': 'AND0003', 'shopId': 'common'}, 'SCGetPaymentInfo')
                    product = next(p for p in products.paymentInfoList if p.paymentId == 'local-p3-red-card')
                    self.assertEqual(product.money, 30.0)
                    order = http('CSGenerateOrder', {'paymentId': product.paymentId, 'money': product.money,
                        'moneyType': product.moneyType, 'channelId': 'AND0003', 'shopId': product.shopId}, 'SCGenerateOrder')
                    self.assertEqual(order.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                    first = http('CSItemUse', {'costItem': {'id': 'item_023', 'num': 1}, 'getItemIndex': 5}, 'SCItemUse')
                    self.assertEqual(sum(x.num for x in getattr(first, 'get').simps if x.id == 'item_053'), 1)
                    second = http('CSItemUse', {'costItem': {'id': 'item_053', 'num': 1}, 'getItemIndex': 2}, 'SCItemUse')
                    acquired = list(getattr(second, 'get').wars)
                    self.assertEqual(len(acquired), 1)
                    self.assertEqual((acquired[0].id, acquired[0].level), ('hero_003', 1))
                    partner = acquired[0].guid
                    paid = self.profile(owner)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),SUM(delta_cents) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='debit';").strip(), '1\t-3000')
                    self.assertEqual(self.sql(f"SELECT status,cents,credited FROM p3_payment_orders WHERE player_id={owner.playerId} AND id='{order.orderId}';").strip(), 'confirmed\t3000\t300')
                    # The sole finite prerequisite: slot access level. Resource/card snapshots remain exact.
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id={owner.playerId};")
                    leveled = self.profile(owner)
                    expected_player = dict(paid[0], level=20)
                    self.assertEqual(leveled, [expected_player, paid[1]])
                    on = {'id': 'hero_003', 'guid': partner, 'formatPos': 2}
                    off = dict(on)
                    http('CSFormationOn', on, 'SCFormationOn')
                    listing([main, partner])
                    before_off = self.profile(owner)
                    response = http('CSFormationOff', off, 'SCFormationOff')
                    off_stamp = stamp
                    after_off = self.profile(owner)
                    self.assertEqual(after_off[0], before_off[0])
                    expected_game = dict(before_off[1], formation=after_off[1]['formation'])
                    self.assertEqual(after_off[1], expected_game, 'Bare hero off may only change formation')
                    self.assertEqual(after_off[1]['warriors'], before_off[1]['warriors'])
                    self.assertEqual([v for v in after_off[1]['warriors'] if v['guid'] == partner],
                                     [v for v in paid[1]['warriors'] if v['guid'] == partner])
                    internal = next(v for v in after_off[1]['formation'] if v['formatPos'] == 2)
                    self.assertFalse(any(t.get('pos', 0) == 0 for t in internal.get('things', [])))
                    listing([main])
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        retries = list(workers.map(lambda _: self.call('CSFormationOff', off, owner, off_stamp, 'SCFormationOff'), range(8)))
                    self.assertTrue(all(r == response for r in retries))
                    self.assertEqual(self.profile(owner), after_off)
                    http('CSFormationOff', off, 'SCFormationOff')  # fresh-timestamp idempotent no-op
                    self.assertEqual(self.profile(owner), after_off)
                    reject({'id': 'hero_003', 'guid': 'unowned-' + uuid.uuid4().hex, 'formatPos': 2})
                    reject({'id': 'hero_001', 'guid': main, 'formatPos': 1})
                    final = self.profile(owner)
                    self.stop()
                    self.start()
                    owner = self.login(identity)
                    self.assertEqual(self.profile(owner), final)
                    listing([main])
                    self.assertEqual(self.call('CSFormationOff', off, owner, off_stamp, 'SCFormationOff'), response)
                    self.assertEqual(self.profile(owner), final)
                    http('CSFormationOn', on, 'SCFormationOn')
                    listing([main, partner])
                    self.assertEqual(self.profile(owner)[1]['warriors'], paid[1]['warriors'])
                    self.assertEqual(self.profile(protected), protected_before)
                    self.assertEqual(self.wallet('snapshot', owner)['balanceCents'], 0)
                    report.update(status='PASS', fullChain=True, actualOrderId=order.orderId,
                        actualWalletDebitedCents=3000, actualPartnerGUID=partner, initial20111Heroes=2,
                        offAndCold20111Heroes=1, reformed20111Heroes=2, emptyInternalSlotRetained=True,
                        actualPartnerInventoryRetained=True, eightOriginalOffRetries=True,
                        forgedGUIDAndMainOffRejected=True, protected1Exact=True, coldRestored=True)
                except BaseException as error:
                    report.update(status='FAIL', failureType=type(error).__name__, failure=str(error))
                    raise
                finally:
                    self.stop()
                    report['ownedAppStopped'] = self.process is None or self.process.poll() is not None
                    self.pool.close()
                    report['ownedManagedAllocationsRemaining'] = len(self.pool.connections)
                    report['seconds'] = round(time.monotonic() - began, 3)
                    output.parent.mkdir(parents=True, exist_ok=True)
                    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
