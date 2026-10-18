"""Prebuilt managed HTTP: wallet -> ten paid orders -> VIP books -> skills.

Only the recovered OutdoorLevel is a finite qualification on a separate demo
account. No VIP, diamonds, boxes, books, crafting XP or skills are inserted.
Four explicitly default-off local VIP offers are enabled only in this owned
isolated schema after their original frozen configuration is checked.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import tempfile
import time
import types
import unittest
import uuid

from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME, CLASSES, parse_http
import test_continued_gameplay_mysql as wire
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_p5_talent_scope_mysql import original_table

ROOT = Path(__file__).resolve().parents[1]
SPECS = [(7, 800, 'item_041', None, None),
         (8, 900, 'item_042', 'item_063', 'LifeSkill_085'),
         (9, 1000, 'item_043', 'item_065', 'SkillFishItem_051'),
         (10, 1100, 'item_044', 'item_064', 'LifeSkill_086')]
PRODUCTS = {f'p3vip{vip}_growth_local': (vip, cost, box, book, skill)
            for vip, cost, box, book, skill in SPECS}
WALLET_DEBIT_KIND = 'debit'
SQL_CONTRACTS = {
    'profiles': {'id', 'uid', 'player', 'game', 'session', 'created_at'},
    'p3_vip_shop_products': {'id', 'min_v_ip', 'cost', 'reward', 'source', 'enabled'},
    'p3_payment_orders': {'id', 'player_id', 'status', 'cents', 'created_at', 'expires_at', 'confirmed_at'},
    'local_wallet_entries': {'player_id', 'key', 'kind', 'delta_cents', 'order_id', 'at', 'balance_after', 'revision'},
    'local_wallet_balances': {'player_id', 'balance_cents', 'revision'},
    'local_wallet_policies': {'id', 'enabled'},
    'local_play_policies': {'id', 'enabled'},
    'local_play_payment_times': {'order_id', 'player_id', 'game_at', 'real_at', 'offset_days'},
    'p8_life_skills': {'player_id', 'skill_id', 'book_id', 'learned_at', 'source'},
}


def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def protocol_preflight():
    vectors = {
        'CSGenerateOrder': {'paymentId': 'djsy.wuyou.andp2000', 'money': 2000.0,
                            'moneyType': 'RMB', 'channelId': 'AND0003', 'shopId': 'common'},
        'CSCheckOrder': {'paymentId': 'djsy.wuyou.andp2000', 'orderId': 'actual-order'},
        'CSShopProductList': {'shopType': 2},
        'CSShopBuy': {'shopType': 2, 'shopItemId': 'p3vip9_growth_local', 'buyCount': 1},
        'CSItemUse': {'costItem': {'id': 'item_043', 'num': 1}},
        'CSCookingLevel': {}, 'CSGemMake': {'gemId': 'MineItem_079', 'num': 1},
        'CSFishCooking': {'fish': {'id': 'FishItem_050', 'num': 1}},
    }
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 100)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Recovered original request changed: ' + name)
    responses = {'SCGenerateOrder': {'orderId', 'transactionData'},
                 'TransactionData': {'payStatus', 'payDiamond', 'freeDiamond'},
                 'SCShopProductList': {'shopItemList'},
                 'ShopItem': {'price', 'discount', 'condition', 'canBuy', 'dayLeftTimes', 'dayTotalTimes'},
                 'SCItemUse': {'get', 'cost'}, 'SCCookingLevel': {'level', 'gemLevel', 'skills'}}
    for name, fields in responses.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Recovered original response changed: ' + name)
    tables = {name: original_table(name) for name in ('GameConfigure', 'Payment', 'Item', 'ItemUse')}
    payment = next(x for x in tables['Payment'] if x['paymentId'] == 'djsy.wuyou.andp2000')
    if float(payment['money']) != 2000 or int(payment['charge']) != 20000 or payment['moneyType'] != 'RMB':
        raise AssertionError('Original 2000-yuan SKU changed')
    outdoor = int(next(x['NameValue'] for x in tables['GameConfigure'] if x['StaticName'] == 'OutdoorLevel'))
    if outdoor < 2:
        raise AssertionError('Original outdoor qualification changed')
    boxes = {}
    items = {row['itemId']: row for row in tables['Item']}
    for _, _, box, book, skill in SPECS:
        rewards = [x for x in tables['ItemUse'] if x['itemId'] == box]
        if items[box]['category'] != '2' or len(rewards) != 1 or rewards[0]['odds'] != '100':
            raise AssertionError('Original deterministic VIP bundle changed: ' + box)
        reward_items = dict(part.split(',') for part in rewards[0]['result'].split(';'))
        if book and reward_items.get(book) != '1':
            raise AssertionError('VIP bundle lost actual skill book: ' + box)
        if any(part in {'diamond', 'player_exp', 'exp'} for part in reward_items):
            raise AssertionError('VIP box changes qualification or diamond accounting')
        boxes[box] = rewards[0]['result']
        if book:
            rows = [x for x in tables['ItemUse'] if x['itemId'] == book]
            if items[book]['category'] != '15' or len(rows) != 1 or rows[0]['result'] != skill or rows[0]['category'] != '15' or rows[0]['odds'] != '100':
                raise AssertionError('Original book/skill mapping changed: ' + book)
    return {'requestVectors': len(vectors), 'responseContracts': len(responses),
            'originalPayment': payment['paymentId'], 'originalCentsPerOrder': 200000,
            'originalChargeDiamondPerOrder': 20000, 'OutdoorLevel': outdoor,
            'boxRewards': boxes, 'learningProtocol': 'CSItemUse/category15 -> p8SkillBook -> SCItemUse'}


def service_contract_preflight(source):
    # Bind each mechanical assumption to the fixed application, before DB allocation.
    required = {
        'service/localWalletService.go': ['Kind: "debit", DeltaCents: -order.Cents',
                                         'Kind: "add", DeltaCents: cents',
                                         'if entry.Kind == "debit"', 'kind, label = "payment", "充值"'],
        'service/p8LifeService.go': ['case *pb.CSItemUse:', 'out, err = s.p8SkillBook(ctx, id, b, stamp)',
                                   'if err = s.cookingOpen(p)', 'out.Cost = proto.Clone(cost)',
                                   'BookID: cost.GetId(), LearnedAt: now', 'receipt.Request', 'receipt.Response'],
        'service/cookingService.go': ['"OutdoorLevel"', 'int64(player.GetLevel()) < level'],
        'service/vipService.go': ['"vip-shop:" + product + ":" + p3Day(now)',
                                'p3ClaimExists(tx, p.GetPlayerId(), "vip-shop:"+cfg.ID, p3Day(now))',
                                'Id: proto.String("diamond"), Num: proto.Int32(int32(cfg.Cost))'],
        'service/paymentService.go': ['s.vipShop(ctx, id, request, LocalPlayNow(ctx))'],
        'service/profileService.go': ['row.Session = make([]byte, 16)', 'return tx.Save(&row).Error'],
        'service/gameService.go': ['case *pb.CSUserLogin:', '"serverTimeNow": LocalPlayNow(ctx).UnixMilli()'],
        'service/localPlayClockService.go': ['GameAt: LocalPlayAt(ctx, real).UnixMilli(), RealAt: real.UnixMilli()',
                                           'never rewrites the persisted payment orders'],
        'db/paymentOrder.go': ['autoCreateTime:false', 'ConfirmedAt int64'],
        'db/localWallet.go': ['func (LocalWalletEntry) TableName() string { return "local_wallet_entries" }'],
    }
    checked = {}
    for relative, needles in required.items():
        path = source / 'hyjs-server' / relative
        content = path.read_text()
        if any(needle not in content for needle in needles):
            raise AssertionError('Fixed service contract differs: ' + relative)
        checked[relative] = sha(path)
    if 'CSLearnLifeSkill' in CLASSES:
        raise AssertionError('Recheck original learning contract after schema change')
    return {'fixedSourceSHA256': checked, 'persistedCreditKind': 'add',
            'persistedDebitKind': WALLET_DEBIT_KIND, 'viewDebitKind': 'payment',
            'coldLogin': 'Account authentication rotates session only; no CSPlayerLogin; profile/game and real timestamps remain exact'}


def preflight():
    contracts = protocol_preflight()
    source = Path(os.environ['HYJS_GROWTH_VIP_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_GROWTH_VIP_BINARY']).resolve()
    proof_path = Path(os.environ['HYJS_GROWTH_VIP_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use the direct fixed qualified candidate')
    manifest = source / 'manifest.json'
    paths = set()
    for entry in json.loads(manifest.read_text())['files']:
        path = (source / entry['path']).resolve()
        if source not in path.parents or path in paths or sha(path) != entry['sha256']:
            raise AssertionError('Exact candidate pin changed: ' + entry['path'])
        paths.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - paths:
        raise AssertionError('Unpinned candidate Go source')
    proof = json.loads(proof_path.read_text())
    if proof.get('qualified') is not True or Path(proof.get('candidate', '')).resolve() != source:
        raise AssertionError('Exact completed root qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Reuse exact qualified app without compiling')
    contracts['services'] = service_contract_preflight(source)
    for name in ('GameConfigure', 'Payment', 'Item', 'ItemUse', 'VipPerks', 'FishSkill', 'MineSkill'):
        relative = 'analysis/tables/Table' + name + '.tsv'
        if sha(source / relative) != sha(ROOT / relative):
            raise AssertionError('Original runtime table differs: ' + name)
    for relative in ('server/codec.py', 'server/protocol/schema.json'):
        if (source / relative).read_bytes() != (ROOT / relative).read_bytes():
            raise AssertionError('Original wire codec differs from fixed runtime')
    constants = (source / 'hyjs-server/constUtil/payment.go').read_text()
    sources = {}
    for vip, _, _, _, _ in SPECS:
        match = re.search(r'^const P3VIP' + str(vip) + r'GrowthSource = (".*")$', constants, re.M)
        if match is None:
            raise AssertionError('Missing exact named local source')
        sources[f'p3vip{vip}_growth_local'] = json.loads(match[1])
    return source, binary, sha(manifest), contracts, sources


@unittest.skipUnless(os.environ.get('HYJS_GROWTH_VIP_MYSQL_TEST') == '1',
                     'Root coordinated isolated prebuilt VIP HTTP window')
class GrowthVIPBooksMySQLTests(unittest.TestCase):
    # Reuse only transport/lifecycle methods from the managed runner. No old
    # root_sql, inherited testcase, compiler, fixture allocator or production app.
    send = transport.send
    login = transport.login
    call = transport.call
    stamped = transport.stamped
    profile = transport.profile
    start = transport.start
    stop = transport.stop
    sql = transport.sql
    json_request = transport.json_request
    wallet_body = transport.wallet_body
    wallet = transport.wallet
    order = transport.order
    tools = transport.tools
    change = transport.change

    def schema_preflight(self):
        tables = {}
        for table, required in SQL_CONTRACTS.items():
            columns = self.sql("SELECT COLUMN_NAME FROM information_schema.COLUMNS "
                               "WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='" + table + "' "
                               "ORDER BY ORDINAL_POSITION;").strip().splitlines()
            self.assertTrue(required <= set(columns), 'Exact runtime SQL schema required: ' + table)
            tables[table] = columns
        self.assertNotIn('min_vip', tables['p3_vip_shop_products'])
        return {'tables': tables, 'minimumVIPColumn': 'min_v_ip',
                'persistedDebitKind': WALLET_DEBIT_KIND}

    def activate_isolated_catalogue(self):
        before = self.sql("SELECT id,min_v_ip,cost,reward,source,enabled FROM p3_vip_shop_products WHERE id IN (" +
            ','.join("'" + key + "'" for key in PRODUCTS) + ') ORDER BY id;').strip().splitlines()
        self.assertEqual(len(before), 4)
        predicates = []
        for line in before:
            key, vip, cost, reward, source, enabled = line.split('\t')
            spec = PRODUCTS[key]
            self.assertEqual((int(vip), int(cost), reward, source, enabled),
                             (spec[0], spec[1], spec[2] + ',1', self.named_sources[key], '0'))
            predicates.append("(id='" + key + f"' AND min_v_ip={spec[0]} AND cost={spec[1]} "
                "AND reward='" + reward + "' AND source=CONVERT(0x" + source.encode().hex() + " USING utf8mb4))")
        self.pool.sql(self.database, 'UPDATE p3_vip_shop_products SET enabled=1 WHERE enabled=0 AND (' + ' OR '.join(predicates) + ');')
        after = self.sql("SELECT id,min_v_ip,cost,reward,source,enabled FROM p3_vip_shop_products WHERE id IN (" +
            ','.join("'" + key + "'" for key in PRODUCTS) + ') ORDER BY id;').strip().splitlines()
        self.assertEqual(after, [line.rsplit('\t', 1)[0] + '\t1' for line in before])
        return [{'id': key, 'minVip': spec[0], 'costDiamond': spec[1],
                 'reward': spec[2] + ',1', 'source': self.named_sources[key]}
                for key, spec in PRODUCTS.items()]

    def test_wallet_ten_real_orders_original_vip_boxes_books_skills_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, manifest_sha, contracts, self.named_sources = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_GROWTH_VIP_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016/vip-books'))
        self.assertFalse(output.exists(), 'Preserve earlier acceptance result')
        self.pool = ManagedMySQL()
        self.process = None
        report = {'status': 'PENDING', 'fullChain': False, 'binarySHA256': sha(binary),
                  'sourceManifestSHA256': manifest_sha, 'sourceRoot': str(self.source),
                  'runnerSHA256': sha(__file__), 'contracts': contracts,
                  'externalPayment': False, 'phoneUI': False, 'productionModified': False,
                  'finitePreparation': 'Only recovered OutdoorLevel on one isolated demo profile; no VIP/diamond/box/book/skill/crafting-level/XP insertions'}
        began = time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-growth-vip-books-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('growth_vip_books')
                    self.database = allocation.database
                    self.env = {k: v for k, v in os.environ.items() if k in {'PATH', 'HOME', 'TMPDIR', 'LANG', 'LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    report['schemaPreflight'] = self.schema_preflight()
                    stamp = 510000

                    def http(login, name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, login, stamp, expected)

                    def once(login, name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.call(name, fields, login, saved, expected), range(8)))
                        self.assertTrue(all(reply == replies[0] for reply in replies))
                        return replies[0], saved

                    def reject(login, name, fields):
                        before = self.profile(login)
                        reply = http(login, name, fields, 'ErrorMessage')
                        self.assertEqual(self.profile(login), before)
                        return reply.msg

                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-vip-protected-' + suffix)
                    self.assertEqual(protected.playerId, 1)
                    http(protected, 'CSPlayerCreate', {'playerName': '保护会员书账号', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-vip-books-' + suffix
                    owner = self.login(uid)
                    self.assertGreater(owner.playerId, 1)
                    http(owner, 'CSPlayerCreate', {'playerName': '原钱包会员书', 'heroId': 'hero_043'}, 'SCPlayerCreate')
                    initial = self.profile(owner)
                    self.assertEqual((initial[0].get('vip', 0), initial[0].get('coin', 0), initial[0].get('diamond', 0)), (0, 0, 0))
                    listed = http(owner, 'CSShopProductList', {'shopType': 2}, 'SCShopProductList')
                    self.assertFalse(set(PRODUCTS) & {row.id for row in listed.shopItemList})
                    reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip9_growth_local', 'buyCount': 1})
                    report['catalogue'] = self.activate_isolated_catalogue()
                    reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip9_growth_local', 'buyCount': 1})
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    protected_status, protected_result = self.json_request('/local-wallet/add', {
                        'playerId': protected.playerId, 'sessionHex': bytes(protected.sessionKey).hex(),
                        'requestId': str(uuid.uuid4()), 'amountCents': 2000000})
                    self.assertEqual(protected_status, 200)
                    self.assertNotEqual(protected_result.get('code'), 0)
                    self.assertEqual(self.profile(protected), protected_before)
                    credit_key = str(uuid.uuid4())
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                        credits = list(workers.map(lambda _: self.wallet('add', owner, requestKey=credit_key, amountCents=2000000), range(8)))
                    self.assertTrue(all(value['balanceCents'] == 2000000 and value['revision'] == 1 for value in credits))
                    orders = []
                    for number in range(1, 11):
                        before = self.profile(owner)[0]
                        stamp += 1
                        saved_stamp = stamp
                        generated = self.order(owner, 'djsy.wuyou.andp2000', saved_stamp)
                        self.assertEqual(generated.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                        duplicate = self.order(owner, 'djsy.wuyou.andp2000', saved_stamp)
                        self.assertEqual(duplicate.orderId, generated.orderId)
                        checked = http(owner, 'CSCheckOrder', {'orderId': generated.orderId, 'paymentId': 'djsy.wuyou.andp2000'}, 'SCCheckOrder')
                        self.assertEqual(checked.transactionData.payStatus, 'PAY_STATUS_SUCCESS')
                        after = self.profile(owner)[0]
                        self.assertEqual(after['chargeDiamond'], number * 20000)
                        view = self.wallet('snapshot', owner)
                        self.assertEqual((view['balanceCents'], view['revision']), (2000000 - number * 200000, number + 1))
                        orders.append({'orderId': generated.orderId, 'amountCents': 200000,
                                       'actualDiamondDelta': after['diamond'] - before.get('diamond', 0),
                                       'chargeDiamond': after['chargeDiamond'], 'vip': after['vip'],
                                       'walletBalanceCents': view['balanceCents']})
                        if number == 5:
                            self.assertEqual(after['vip'], 9)
                            reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip10_growth_local', 'buyCount': 1})
                    paid, _ = self.profile(owner)
                    self.assertEqual((paid['vip'], paid['chargeDiamond']), (10, 200000))
                    initial_paid_diamond = paid['diamond']
                    self.assertEqual(len({order['orderId'] for order in orders}), 10)
                    self.assertEqual(self.sql(f"SELECT COUNT(*),COALESCE(SUM(cents),0) FROM p3_payment_orders WHERE player_id={owner.playerId} AND status='confirmed';").strip(), '10\t2000000')
                    self.assertEqual(self.sql(f"SELECT COUNT(*),COALESCE(SUM(delta_cents),0) FROM local_wallet_entries WHERE player_id={owner.playerId} AND kind='{WALLET_DEBIT_KIND}';").strip(), '10\t-2000000')
                    self.assertEqual(self.sql(f"SELECT COUNT(*),COALESCE(SUM(delta_cents),0) FROM local_wallet_entries WHERE player_id={owner.playerId};").strip(), '11\t0')
                    real_time_queries = {
                        'orders': f'SELECT id,created_at,expires_at,confirmed_at FROM p3_payment_orders WHERE player_id={owner.playerId} ORDER BY id;',
                        'ledger': f'SELECT `key`,kind,delta_cents,balance_after,at FROM local_wallet_entries WHERE player_id={owner.playerId} ORDER BY revision;',
                        'paymentClock': f'SELECT order_id,game_at,real_at,offset_days FROM local_play_payment_times WHERE player_id={owner.playerId} ORDER BY order_id;',
                    }
                    real_times = {key: self.sql(query).strip() for key, query in real_time_queries.items()}
                    self.assertEqual(len(real_times['orders'].splitlines()), 10)
                    self.assertEqual(len(real_times['ledger'].splitlines()), 11)
                    self.assertEqual(len(real_times['paymentClock'].splitlines()), 10)
                    for row in real_times['orders'].splitlines():
                        _, created, expires, confirmed = row.split('\t')
                        self.assertTrue(0 < int(created) <= int(confirmed) < int(expires))
                    listed = http(owner, 'CSShopProductList', {'shopType': 2}, 'SCShopProductList')
                    available = {row.id: row for row in listed.shopItemList if row.id in PRODUCTS}
                    self.assertEqual(set(available), set(PRODUCTS))
                    records, study_receipts, book_ids = [], [], []
                    total_cost = 0
                    for product, (vip, price, box, book, skill) in PRODUCTS.items():
                        row = available[product]
                        self.assertEqual((row.condition.type, row.condition.value, row.price[0].id, row.price[0].num,
                                          row.discount[0].id, row.discount[0].num, row.canBuy, row.dayLeftTimes, row.dayTotalTimes),
                                         ('vip', vip, 'diamond', price, 'diamond', price, 1, 1, 1))
                        request = {'shopType': 2, 'shopItemId': product, 'buyCount': 1}
                        stamp += 1
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            replies = list(workers.map(lambda _: self.stamped('CSShopBuy', request, owner, stamp), range(8)))
                        successful = [reply for opcode, reply in replies if opcode == BY_NAME['SCShopBuy']]
                        self.assertEqual((len(successful), sum(opcode == 10001 for opcode, _ in replies)), (1, 7))
                        self.assertEqual([(item.id, item.num) for item in successful[0].cost], [('diamond', price)])
                        total_cost += price
                        reject(owner, 'CSShopBuy', request)
                        opened, _ = once(owner, 'CSItemUse', {'costItem': {'id': box, 'num': 1}}, 'SCItemUse')
                        if book:
                            self.assertEqual(sum(item.num for item in getattr(opened, 'get').simps if item.id == book), 1)
                            book_ids.append(book)
                        else:
                            self.assertFalse(any(item.id in {'item_063', 'item_064', 'item_065'} for item in getattr(opened, 'get').simps))
                        records.append({'vip': vip, 'product': product, 'priceDiamond': price, 'box': box,
                                        'book': book, 'skill': skill, 'parallelBuySuccess': 1,
                                        'parallelBuyQuotaRejected': 7, 'boxOpenRetriesExact': 8})
                    # Outdoor access is the sole finite profile input. First prove
                    # original level1 rejection leaves the actual learned book held.
                    reject(owner, 'CSItemUse', {'costItem': {'id': 'item_063', 'num': 1}})
                    before_outdoor = self.profile(owner)
                    outdoor = contracts['OutdoorLevel']
                    self.pool.sql(self.database, f"UPDATE profiles SET player=JSON_SET(player,'$.level',{outdoor}) WHERE id={owner.playerId} AND uid LIKE 'demo-vip-books-%';")
                    after_outdoor = self.profile(owner)
                    self.assertEqual(after_outdoor[1], before_outdoor[1])
                    self.assertEqual({k: v for k, v in after_outdoor[0].items() if k != 'level'},
                                     {k: v for k, v in before_outdoor[0].items() if k != 'level'})
                    for _, _, _, book, skill in SPECS:
                        if not book:
                            continue
                        request = {'costItem': {'id': book, 'num': 1}}
                        learned, saved_stamp = once(owner, 'CSItemUse', request, 'SCItemUse')
                        study_receipts.append((request, saved_stamp, learned))
                        self.assertEqual((learned.cost.id, learned.cost.num), (book, 1))
                        self.assertIn(skill, http(owner, 'CSCookingLevel', {}, 'SCCookingLevel').skills)
                        reject(owner, 'CSItemUse', request)
                    state = http(owner, 'CSCookingLevel', {}, 'SCCookingLevel')
                    expected_skills = {'LifeSkill_085', 'LifeSkill_086', 'SkillFishItem_051'}
                    self.assertEqual((state.level, state.gemLevel, set(state.skills)), (1, 1, expected_skills))
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM p8_life_skills WHERE player_id={owner.playerId};').strip(), '3')
                    gates = []
                    for name, fields in [('CSGemMake', {'gemId': 'MineItem_079', 'num': 1}),
                                         ('CSGemMake', {'gemId': 'MineItem_080', 'num': 1}),
                                         ('CSFishCooking', {'fish': {'id': 'FishItem_050', 'num': 1}})]:
                        gates.append({'request': name, 'error': reject(owner, name, fields)})
                    # A new actual play date gives one new VIP7 allocation. Moving
                    # back restores the existing claim key rather than a free copy.
                    self.assertEqual(self.change(owner, 'nextDay')['offsetDays'], 1)
                    http(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1}, 'SCShopBuy')
                    total_cost += 800
                    reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    self.assertEqual(self.change(owner, 'previousDay')['offsetDays'], 0)
                    reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip7_growth_local', 'buyCount': 1})
                    self.assertEqual(self.change(owner, 'reset')['offsetDays'], 0)
                    self.assertEqual(self.profile(owner)[0]['diamond'], initial_paid_diamond - total_cost)
                    before_cold = self.profile(owner)
                    wallet_before_cold = self.wallet('snapshot', owner)
                    self.stop()
                    self.start()
                    owner = self.login(uid)
                    self.assertEqual(self.profile(owner), before_cold)
                    self.assertEqual(self.wallet('snapshot', owner), wallet_before_cold)
                    self.assertEqual(self.tools(owner, 'time/state')['offsetDays'], 0)
                    self.assertEqual({key: self.sql(query).strip() for key, query in real_time_queries.items()}, real_times,
                                     'Play-date movement and account cold login must preserve actual money/order time')
                    for request, saved_stamp, learned in study_receipts:
                        self.assertEqual(self.call('CSItemUse', request, owner, saved_stamp, 'SCItemUse'), learned)
                        self.assertEqual(self.profile(owner), before_cold)
                    reject(owner, 'CSShopBuy', {'shopType': 2, 'shopItemId': 'p3vip9_growth_local', 'buyCount': 1})
                    cold = http(owner, 'CSCookingLevel', {}, 'SCCookingLevel')
                    self.assertEqual((cold.level, cold.gemLevel, set(cold.skills)), (1, 1, expected_skills))
                    self.assertEqual(self.profile(protected), protected_before)
                    final_game = self.profile(owner)[1]
                    book_balances = {book: sum(item['num'] for item in (final_game.get('items') or []) if item['id'] == book)
                                     for book in book_ids}
                    self.assertEqual(set(book_balances.values()), {0})
                    self.assertEqual(sum(item['num'] for item in (final_game.get('items') or []) if item['id'] == 'item_041'), 1)
                    self.assertEqual(self.sql(f'SELECT COUNT(*) FROM local_wallet_balances WHERE player_id={protected.playerId};').strip(), '0')
                    self.assertEqual(self.sql('SELECT DISTINCT ENGINE FROM information_schema.TABLES '
                        f"WHERE TABLE_SCHEMA='{self.database}' AND TABLE_NAME IN ('profiles','p3_payment_orders','local_wallet_entries','p8_life_skills');").split(), ['InnoDB'])
                    report.update(status='PASS', fullChain=True, actualWallet={'creditCents': 2000000,
                        'paymentCents': 2000000, 'balanceCents': wallet_before_cold['balanceCents'],
                        'revision': wallet_before_cold['revision'], 'ledgerRows': len(wallet_before_cold['entries'])},
                        actualOrders=orders, vip9AfterOrders=5, vip10AfterOrders=10,
                        actualRealTimeRows={key: value.splitlines() for key, value in real_times.items()},
                        actualChargeDiamond=200000, actualDiamondFromOrders=initial_paid_diamond,
                        actualVIPPurchases=records, paidVIPBoxesDiamond=total_cost,
                        actualBooks=book_ids, actualSkills=sorted(cold.skills),
                        actualBookBalancesAfterLearning=book_balances, actualPersistedSkillRows=3,
                        freshCookingLevel=cold.level, freshGemLevel=cold.gemLevel,
                        freshRecipeRejections=gates, dailyNewDateAndRewind=True,
                        coldRestarts=1, originalSkillReceiptsExactAfterCold=True,
                        protected1Exact=True, defaultOffVerified=True, lowVIPAndOutdoorRejected=True,
                        finalDiamond=self.profile(owner)[0]['diamond'],
                        unopenedNextDateBox={'id': 'item_041', 'num': 1},
                        scope='four named local VIP7/8/9/10 offers at original client shop entry; original boxes/books/skill consumers; no full high-level recipe-source claim')
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
