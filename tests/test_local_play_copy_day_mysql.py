"""Prebuilt HTTP copy quotas across personal previous/next day and cold.

Root supplies SHA-bound actual player/game and related P9 rows, no-Run snapshots. They are cloned
into one disposable new identity, with no added rewards/stars/resources/winner.
The runner never connects to the formal database or reads account/session fields.
No compiler/phone/live writes. Main entry is Enter/Leave, never a forged fight End.
"""
import concurrent.futures
import base64
import copy
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

from hyjs_managed_mysql import ManagedMySQL
from test_growth_recruit_mysql import GrowthRecruitMySQLTests as transport
from test_local_play_training_rebate_mysql import sha
from test_p5_talent_scope_mysql import original_table
from server.codec import CLASSES, parse_http
import test_continued_gameplay_mysql as wire

ROOT = Path(__file__).resolve().parents[1]
MISSION = {'chapterId': '1', 'missionId': '5'}

# Only this bounded sweep/cold closure is imported. Old grants, deferred claims,
# interaction windows and mutation receipts remain facts of the original owner.
P9_COLUMNS = {
    'p9_pet_instances': ('guid', 'player_id', 'pet_id', 'data_hex', 'created_at', 'source'),
    'p9_pet_talent_allocations': ('guid', 'player_id', 'policy_hash', 'draws_hex', 'source'),
    'p9_pet_investments': ('guid', 'player_id', 'costs_hex', 'donors_hex', 'source'),
}


def p9_dependencies(snapshot, facts):
    path = Path(os.environ['HYJS_COPY_DAY_P9_SNAPSHOT']).resolve()
    if not path.is_relative_to(ROOT/'analysis') or sha(path) != os.environ['HYJS_COPY_DAY_P9_SNAPSHOT_SHA256']:
        raise AssertionError('Root read-only related P9 snapshot SHA required')
    dependency = json.loads(path.read_bytes())
    if set(dependency) != {'sourcePlayerID', 'profileSnapshotSHA256', *P9_COLUMNS}:
        raise AssertionError('Only exact related P9 business columns accepted')
    if dependency['sourcePlayerID'] != facts['sourcePlayerID'] or dependency['profileSnapshotSHA256'] != sha(snapshot):
        raise AssertionError('P9 export must bind this exact actual profile snapshot')
    projected = {v['key'][len('p9.pet:'):]: base64.b64decode(v['value'], validate=True)
                 for v in facts['game'].get('kv', []) if v['key'].startswith('p9.pet:') and v.get('value')}
    guarded = {t['guid']: t['id'] for camp in facts['game'].get('formation', [])
               if any(t.get('pos', 0) == 0 and t.get('id', '').startswith('hero_') for t in camp.get('things', []))
               for t in camp.get('things', []) if t.get('pos') == 7}
    if not guarded or not set(guarded) <= set(projected):
        raise AssertionError('Actual guard must keep its nonempty P9 projection')
    pets = {}
    for table, columns in P9_COLUMNS.items():
        rows = dependency[table]
        if not isinstance(rows, list):
            raise AssertionError('P9 export rows must be arrays')
        seen = set()
        for row in rows:
            if set(row) != set(columns) or row['player_id'] != facts['sourcePlayerID'] or not row['source']:
                raise AssertionError('P9 owner/source/column contract differs: '+table)
            guid = row['guid']
            if guid not in projected or guid in seen:
                raise AssertionError('P9 GUID must be unique and projected by the source profile')
            seen.add(guid)
            for key in columns:
                if key.endswith('_hex'):
                    bytes.fromhex(row[key])
            if table == 'p9_pet_instances':
                raw = bytes.fromhex(row['data_hex'])
                pet = CLASSES['Pet'].FromString(raw)
                if raw != projected[guid] or pet.guid != guid or pet.id != row['pet_id'] or pet.level < 1 or pet.exp < 0 or not pet.HasField('eat') or not pet.HasField('breached'):
                    raise AssertionError('P9 row must byte-match source projection; do not join snapshots from different times')
                if guid in guarded and guarded[guid] != pet.id:
                    raise AssertionError('Guard ID must match actual independent P9 instance')
                pets[guid] = pet
            elif table == 'p9_pet_talent_allocations':
                draws = json.loads(bytes.fromhex(row['draws_hex']))
                if guid not in pets or draws != list(pets[guid].randPropList) or len(row['policy_hash']) != 64:
                    raise AssertionError('Existing frozen P9 random choices must match actual pet')
            else:
                if guid not in pets:
                    raise AssertionError('Investment must have a live related pet')
                for key in ['costs_hex', 'donors_hex']:
                    json.loads(bytes.fromhex(row[key]))
        if table == 'p9_pet_instances' and seen != set(projected):
            raise AssertionError('Export every nonempty actual P9 projection; never invent or drop a pet')
    return path, dependency


def clone_p9_rows(pool, database, owner_id, dependency):
    for table, columns in P9_COLUMNS.items():
        for row in dependency[table]:
            names, values = [], []
            for key in columns:
                names.append(key[:-4] if key.endswith('_hex') else key)
                if key == 'player_id':
                    values.append(str(int(owner_id)))
                elif key == 'created_at':
                    values.append(str(int(row[key])))
                elif key.endswith('_hex'):
                    values.append("X'"+bytes.fromhex(row[key]).hex()+"'")
                else:
                    values.append("CONVERT(X'"+row[key].encode().hex()+"' USING utf8mb4)")
            pool.sql(database, 'INSERT INTO '+table+' ('+','.join(names)+') VALUES ('+','.join(values)+');')


def contracts():
    vectors = {'CSCopyPassedInfo': {}, 'CSCopyEnter': MISSION, 'CSCopyLeave': {},
               'CSCopySweep': MISSION, 'CSCopyGetResetInfo': MISSION,
               'CSCopyResetChallengeCount': dict(MISSION, cost={'id': 'diamond', 'num': 20})}
    for name, fields in vectors.items():
        raw = wire.encode_original_http(name, fields, 2, b'0' * 16, 95001)
        if parse_http(raw).decode() != CLASSES[name](**fields):
            raise AssertionError('Original copy frame mismatch: ' + name)
    for name, fields in {'SCCopyPassedInfo': {'passedCopyInfo'}, 'SCCopyEnter': {'sceneName', 'taskId', 'jobList'},
                         'SCCopySweep': {'gain', 'cost'}, 'SCCopyGetResetInfo': {'cost', 'remainResetCount'},
                         'SCCopyResetChallengeCount': {'currentSimpleInfo'}}.items():
        if not fields <= set(CLASSES[name].DESCRIPTOR.fields_by_name):
            raise AssertionError('Original copy response mismatch: ' + name)
    return len(vectors)


def preflight():
    vector_count = contracts()
    source = Path(os.environ['HYJS_COPY_DAY_SOURCE']).resolve()
    binary = Path(os.environ['HYJS_COPY_DAY_BINARY']).resolve()
    record = Path(os.environ['HYJS_COPY_DAY_VALIDATION']).resolve()
    if source.parent != ROOT / 'build/candidates':
        raise AssertionError('Use a direct immutable qualified candidate')
    manifest = source / 'manifest.json'
    pinned = set()
    for row in json.loads(manifest.read_bytes())['files']:
        path = (source / row['path']).resolve()
        if not path.is_relative_to(source) or path in pinned or sha(path) != row['sha256']:
            raise AssertionError('Candidate source pin differs: ' + row['path'])
        pinned.add(path)
    if set((source / 'hyjs-server').rglob('*.go')) - pinned:
        raise AssertionError('Candidate has unpinned Go')
    proof = json.loads(record.read_bytes())
    if proof.get('qualified') is not True or (ROOT / proof['candidate']).resolve() != source:
        raise AssertionError('Root completed qualification required')
    if sha(binary) != (proof.get('binarySHA256') or proof.get('hyjs-serverSHA256')):
        raise AssertionError('Exact same qualified binary required; never compile here')
    for name in ['server/codec.py', 'server/protocol/schema.json', *['analysis/tables/Table'+x+'.tsv'
            for x in ['CopyMission', 'CopyChapter', 'VipPerks', 'GameConfigure']]]:
        if sha(source / name) != sha(ROOT / name):
            raise AssertionError('Original contract/table differs: ' + name)
    for path in ['copyService.go', 'copySweepService.go', 'copyResetService.go']:
        if 's.refreshCopyCalendar(ctx, tx, id,' not in (source / 'hyjs-server/service' / path).read_text():
            raise AssertionError('Personal copy day integration missing: ' + path)
    if 'copy-day:' not in (source / 'hyjs-server/service/localPlayCopyDayService.go').read_text():
        raise AssertionError('Durable personal copy day archive missing')
    if 'applyLocalPlayRecoveryJump' not in (source / 'hyjs-server/service/localPlayClockService.go').read_text():
        raise AssertionError('Qualified R3 new-personal-day recovery required')
    snapshot = Path(os.environ['HYJS_COPY_DAY_SNAPSHOT']).resolve()
    if not snapshot.is_relative_to(ROOT / 'analysis') or sha(snapshot) != os.environ['HYJS_COPY_DAY_SNAPSHOT_SHA256']:
        raise AssertionError('Root actual read-only source snapshot SHA required')
    facts = json.loads(snapshot.read_bytes())
    if not {'sourcePlayerID', 'player', 'game'} <= set(facts) or set(facts) - {'sourcePlayerID', 'scope', 'player', 'game'} or facts['sourcePlayerID'] != 4 or facts['game'].get('run') is not None:
        raise AssertionError('Only root sourcePlayerID4 plus player/game columns with Run=null accepted')
    player, game = facts['player'], facts['game']
    if any(k in player or k in game for k in ['session', 'sessionHex', 'sessionKey', 'password', 'account', 'uid']):
        raise AssertionError('Snapshot must not contain account/session credentials')
    info = [x for x in game.get('copy', {}).get('passedCopyInfo', [])
            if x.get('chapterId') == '1' and x.get('missionId') == '5']
    if len(info) != 1 or info[0].get('star') != 5:
        raise AssertionError('Actual finite original mission5 five-star snapshot required; no invented win')
    row = next(x for x in original_table('CopyMission') if x['MissionId'] == '5')
    if row['ChapterId'] != '1':
        raise AssertionError('Original mission5 chapter changed')
    limit, power = int(row['ChallengeCount']), int(row['PowerCost'])
    vip = next(x for x in original_table('VipPerks') if int(x['vip']) == player['vip'])
    quota = int(vip['numPerk1'])
    cfg = {x['StaticName']: x['NameValue'] for x in original_table('GameConfigure')}
    prices = list(map(int, cfg['CopyResetDiamondCost'].split(';')))
    if limit < 3 or power <= 0 or quota < 2 or len(prices) != 4:
        raise AssertionError('Actual finite VIP must expose the original next reset quote')
    # D0 exhaust/reset/sweep; D-1 Enter/Leave plus two sweeps; D+1 one
    # sweep. For original mission5: 3+1+1+2+1=8 actions/80 actual stamina.
    minimum_stamina = power * (limit + 5)
    if player.get('stamina', 0) < minimum_stamina or player.get('diamond', 0) < prices[0]:
        raise AssertionError('Actual finite snapshot lacks original stamina/reset budget; do not top up via SQL')
    if player['level'] < int(cfg['FightTenLevel']) or player['vip'] < int(cfg['FightTenVIPLevel']):
        raise AssertionError('Actual source must qualify for original sweep')
    p9_path, dependencies = p9_dependencies(snapshot, facts)
    return source, binary, snapshot, facts, dict(vectors=vector_count, limit=limit, power=power,
        vipQuota=quota, prices=prices, minimumStamina=minimum_stamina), p9_path, dependencies


@unittest.skipUnless(os.environ.get('HYJS_COPY_DAY_MYSQL_TEST') == '1', 'Root prebuilt managed window only')
class LocalPlayCopyDayMySQLTests(unittest.TestCase):
    send, login, call = transport.send, transport.login, transport.call
    stamped, profile, start, stop, sql = transport.stamped, transport.profile, transport.start, transport.stop, transport.sql
    json_request, tools = transport.json_request, transport.tools

    def test_original_copy_personal_days_reset_quotes_rewind_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'), '1')
        self.source, binary, snapshot, facts, contract, p9_path, dependencies = preflight()
        self.binary = str(binary)
        output = Path(os.environ['HYJS_COPY_DAY_OUTPUT']).resolve()
        self.assertTrue(output.is_relative_to(ROOT / 'analysis/growth-first-20261016'))
        self.assertFalse(output.exists(), 'Preserve prior attempts')
        report = dict(status='PENDING', fullChain=False, runnerSHA256=sha(__file__), binarySHA256=sha(binary),
            sourceManifestSHA256=sha(self.source/'manifest.json'), sourceSnapshotSHA256=sha(snapshot),
            sourceP9SnapshotSHA256=sha(p9_path), relatedP9Rows={k:len(dependencies[k]) for k in P9_COLUMNS},
            contracts=contract, finitePreparation='Actual existing player/game plus related P9 instances/allocation/investment cloned to isolated new identity; five stars/VIP/resources inherited, not newly earned',
            noAddedResources=True, noFakeBattleEnd=True, phoneUI=False, formalDBConnections=0,
            formalProtectedIDs=[1, 4, 7], formalDBWrites=0,
            resetLedgerPreparation='New isolated identity starts a new reset-day ledger; actual source VIP/profile inherited, formal reset ledgers not cloned')
        began = time.monotonic()
        self.pool, self.process = ManagedMySQL(), None
        with tempfile.TemporaryDirectory(prefix='hyjs-copy-day-') as temporary:
            self.app_temp = types.SimpleNamespace(name=temporary)
            with open('/tmp/hyjs-isolated-mysql.lock', 'a+') as lock:
                fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
                try:
                    allocation = self.pool.allocate('copy_day')
                    self.database = allocation.database
                    self.env = {k:v for k,v in os.environ.items() if k in {'PATH','HOME','TMPDIR','LANG','LC_ALL'}}
                    self.env.update(allocation.env())
                    self.env.update(TZ='Asia/Shanghai', GOMEMLIMIT='768MiB', GOMAXPROCS='2')
                    self.start()
                    self.pool.sql(self.database, 'UPDATE local_wallet_policies SET enabled=1 WHERE id=1; UPDATE local_play_policies SET enabled=1 WHERE id=1;')
                    suffix = secrets.token_hex(5)
                    protected = self.login('demo-copy-day-protected-'+suffix)
                    self.assertEqual(protected.playerId, 1)
                    self.call('CSPlayerCreate', {'playerName':'日历保护','heroId':'hero_001'}, protected, 95001, 'SCPlayerCreate')
                    protected_before = self.profile(protected)
                    uid = 'demo-copy-day-'+suffix
                    owner = self.login(uid)
                    self.call('CSPlayerCreate', {'playerName':'副本日期','heroId':'hero_001'}, owner, 95002, 'SCPlayerCreate')
                    fresh, _ = self.profile(owner)
                    cloned = copy.deepcopy(facts)
                    cloned['player']['playerId'] = fresh['playerId']
                    cloned['player']['name'] = fresh['name']
                    encoded = [json.dumps(cloned[k],ensure_ascii=False,separators=(',',':')).encode().hex() for k in ['player','game']]
                    self.assertGreater(owner.playerId, 1)
                    self.pool.sql(self.database, "UPDATE profiles SET player=CONVERT(0x"+encoded[0]+" USING utf8mb4),game=CONVERT(0x"+encoded[1]+" USING utf8mb4) WHERE id="+str(int(owner.playerId))+" AND uid='"+uid+"';")
                    clone_p9_rows(self.pool, self.database, owner.playerId, dependencies)
                    self.assertEqual(self.profile(owner), [cloned['player'],cloned['game']])
                    # 克隆档保留已提交回执；新调用沿真实毫秒戳继续，不能回到旧测试小戳。
                    stamp = max(int(time.time() * 1000), int(cloned['game'].get('copy_sweep_receipt', {}).get('timestamp', 0)) + 1000)
                    def http(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        return self.call(name, fields, owner, stamp, expected)
                    def once(name, fields, expected):
                        nonlocal stamp
                        stamp += 1
                        saved = stamp
                        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as workers:
                            rows = list(workers.map(lambda _:self.call(name,fields,owner,saved,expected),range(8)))
                        self.assertTrue(all(x == rows[0] for x in rows))
                        return rows[0], saved
                    def used():
                        rows = http('CSCopyPassedInfo',{},'SCCopyPassedInfo').passedCopyInfo
                        return next(x.challengeTimes for x in rows if x.chapterId=='1' and x.missionId=='5')
                    def quote():
                        q = http('CSCopyGetResetInfo',MISSION,'SCCopyGetResetInfo')
                        self.assertEqual(len(q.cost),1)
                        self.assertEqual(q.cost[0].id,'diamond')
                        return q.remainResetCount,q.cost[0].num
                    changes = []
                    highest_day = 0
                    def change(action):
                        nonlocal highest_day
                        before = self.profile(owner)[0]
                        key = str(uuid.uuid4())
                        result = self.tools(owner,'time/change',action=action,requestId=key)
                        after = self.profile(owner)[0]
                        self.assertEqual(self.tools(owner,'time/change',action=action,requestId=key),result)
                        self.assertEqual(self.profile(owner)[0],after, 'Replayed date request must not recover twice')
                        if result['offsetDays'] > highest_day:
                            self.assertEqual(after['stamina'],max(before['stamina'],100))
                            self.assertEqual(after['energy'],max(before['energy'],30))
                        else:
                            self.assertEqual((after['stamina'],after['energy']),(before['stamina'],before['energy']))
                        highest_day = max(highest_day,result['offsetDays'])
                        changes.append(dict(action=action,offset=result['offsetDays'],revision=result['revision'],
                            staminaBefore=before['stamina'],staminaAfter=after['stamina'],
                            energyBefore=before['energy'],energyAfter=after['energy']))
                        return result
                    total_sweeps, paid = 0, 0
                    last_sweep = None
                    def sweep():
                        nonlocal total_sweeps,last_sweep
                        r,saved = once('CSCopySweep',MISSION,'SCCopySweep')
                        self.assertEqual([(x.id,x.num) for x in r.cost],[('stamina',contract['power'])])
                        self.assertTrue(r.gain.simps)
                        total_sweeps += 1
                        last_sweep = (r,saved)
                    def exhaust():
                        for _ in range(contract['limit']-used()): sweep()
                        self.assertEqual(used(),contract['limit'])
                        before = self.profile(owner)
                        http('CSCopySweep',MISSION,'ErrorMessage')
                        http('CSCopyEnter',MISSION,'ErrorMessage')
                        self.assertEqual(self.profile(owner),before)
                    resets=[]
                    def reset(expected_price):
                        nonlocal paid
                        before=self.profile(owner)
                        http('CSCopyResetChallengeCount',dict(MISSION,cost={'id':'diamond','num':1}),'ErrorMessage')
                        self.assertEqual(self.profile(owner),before)
                        request=dict(MISSION,cost={'id':'diamond','num':expected_price})
                        r,saved=once('CSCopyResetChallengeCount',request,'SCCopyResetChallengeCount')
                        self.assertEqual(r.currentSimpleInfo.challengeTimes,0)
                        self.assertEqual(self.profile(owner)[0]['diamond'],before[0]['diamond']-expected_price)
                        paid += expected_price
                        resets.append((request,r,saved))
                    quota,prices=contract['vipQuota'],contract['prices']
                    self.assertEqual(quote(),(quota,prices[0]))
                    exhaust(); reset(prices[0]); sweep()
                    self.assertEqual((used(),quote()),(1,(quota-1,prices[1])))
                    change('previousDay')
                    self.assertEqual((used(),quote()),(0,(quota,prices[0])))
                    entered=http('CSCopyEnter',MISSION,'SCCopyEnter')
                    self.assertTrue(entered.sceneName)
                    self.assertIsNotNone(self.profile(owner)[1].get('run'))
                    http('CSCopyLeave',{},'SCCopyLeave')
                    self.assertEqual(used(),0)
                    sweep(); sweep()
                    self.assertEqual(used(),2)
                    change('nextDay')
                    self.assertEqual((used(),quote()),(1,(quota-1,prices[1])))
                    change('nextDay')
                    self.assertEqual((used(),quote()),(0,(quota,prices[0])))
                    sweep()
                    change('previousDay')
                    self.assertEqual((used(),quote()),(1,(quota-1,prices[1])))
                    final=self.profile(owner)
                    self.assertEqual(final[0]['diamond'],cloned['player']['diamond']-paid)
                    self.assertIsNone(final[1].get('run'))
                    self.stop(); self.start(); owner=self.login(uid)
                    self.assertEqual(self.profile(owner),final)
                    self.assertEqual((used(),quote()),(1,(quota-1,prices[1])))
                    # Latest successful sweep may replay across days, with no reward
                    # or challenge increment. A reset receipt must not clear the count again.
                    self.call('CSCopySweep',MISSION,owner,last_sweep[1],'SCCopySweep')
                    self.assertEqual(self.profile(owner),final)
                    self.call('CSCopyResetChallengeCount',resets[-1][0],owner,resets[-1][2],'SCCopyResetChallengeCount')
                    self.assertEqual(self.profile(owner),final)
                    change('previousDay')
                    self.assertEqual((used(),quote()),(2,(quota,prices[0])))
                    change('nextDay'); change('nextDay')
                    self.assertEqual((used(),quote()),(1,(quota,prices[0])))
                    self.assertEqual(self.profile(protected),protected_before)
                    report.update(status='PASS',fullChain=True,actualSweeps=total_sweeps,actualResetDiamond=paid,
                        resetPrices=[prices[0]],calendarChanges=changes,eightSameStampOnce=True,
                        mainEntryLeave=True,rewindRestoredUsageAndVIPQuote=True,coldExact=True,
                        newDayResourceRecoveryOnce=True,revisitDoesNotRecoverAgain=True,
                        protectedIsolated1Exact=True,noFormalConnectionProtects147=True)
                except BaseException as error:
                    report.update(status='FAIL',failureType=type(error).__name__,failure=str(error))
                    raise
                finally:
                    self.stop(); self.pool.close()
                    report['seconds']=round(time.monotonic()-began,3)
                    report['ownedManagedAllocationsRemaining']=len(self.pool.connections)
                    output.parent.mkdir(parents=True,exist_ok=True)
                    output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__': unittest.main()
