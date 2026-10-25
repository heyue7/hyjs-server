"""Original 15 enabled awakenings: fragment acquisition to fusion100 and reset."""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import types
import http.client
import unittest

from server.codec import BY_NAME
import test_p5_talent_scope_mysql as shared
from hyjs_managed_mysql import ManagedMySQL


@unittest.skipUnless(os.environ.get('HYJS_P5_AWAKE_LIFECYCLE_MYSQL_TEST') == '1', 'Explicit isolated awakening lifecycle test')
class AwakeLifecycleMySQLTests(unittest.TestCase):
    send = shared.TalentScopeMySQLTests.send
    stamped = shared.TalentScopeMySQLTests.stamped
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    write_profile = shared.TalentScopeMySQLTests.write_profile

    @classmethod
    def setUpClass(cls):
        cls.lock = open('/tmp/hyjs-isolated-mysql.lock', 'a+')
        # A retained QA runtime can hold the file while its owner grants a
        # short serial regression window. Never forcibly unlock that runtime.
        if os.environ.get('HYJS_AWAKE_COORDINATED_WINDOW') != 'guild-20261026-five-minutes':
            fcntl.flock(cls.lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        cls.addClassCleanup(cls.lock.close)
        cls.pool = ManagedMySQL()
        cls.addClassCleanup(cls.pool.close)
        connection = cls.pool.allocate('p5_talent_awake')
        cls.database = connection.database
        cls.env = dict(os.environ, **connection.env(), HYJS_GAME_ACCOUNTS_DATABASE='',
                       GOMAXPROCS='2', GOMEMLIMIT='768MiB')
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-awake-audit-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = os.environ['HYJS_AWAKE_AUDIT_BINARY']
        cls.source_shas = {str(p.relative_to(shared.ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
                           for p in sorted((shared.ROOT / 'hyjs-server').rglob('*.go'))}
        cls.timings = []
        cls.registered = set()
        cls.config = Path(cls.temp.name) / 'config.yaml'
        cls.config.write_text('logConfig:\n  logLevel: error\n  maxSize: 2\n  maxBackups: 1\n  maxAge: 1\ngame:\n  driver: mysql\n  serverId: 1\n')
        cls.env['HYJS_CONFIG_FILE'] = str(cls.config)

    def sql(self, text):
        return self.pool.sql(self.database, text)

    def login(self, uid):
        if uid not in self.registered:
            c = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=3)
            try:
                c.request('POST', '/game-account/register', json.dumps({'username': uid, 'nickname': '觉醒验收',
                    'password': 'local-test-password', 'confirmPassword': 'local-test-password'}), {'Content-Type': 'application/json'})
                response = c.getresponse(); body = json.loads(response.read())
                self.assertEqual((response.status, body.get('code')), (200, 0), body.get('msg'))
            finally:
                c.close()
            self.registered.add(uid)
        return shared.TalentScopeMySQLTests.login(self, uid)

    def start(self):
        log = open(Path(self.temp.name) / 'app.log', 'ab')
        self.process = subprocess.Popen([self.binary, '--http-port', '28987', '--tcp-port', '28988',
            '--bind', '127.0.0.1', '--advertise-host', '127.0.0.1', '--tables', str(shared.ROOT / 'analysis/tables'),
            '--log-file', str(Path(self.temp.name) / 'game.log')], cwd=shared.ROOT / 'hyjs-server',
            env=self.env, stdout=log, stderr=log)
        log.close()
        self.service = types.SimpleNamespace(http_port=28987, tcp_port=28988)
        for _ in range(240):
            if self.process.poll() is not None:
                self.fail(self.pool.redact((Path(self.temp.name) / 'app.log').read_text()[-2500:]))
            try:
                c = http.client.HTTPConnection('127.0.0.1', 28987, timeout=.5)
                c.request('GET', '/health'); response = c.getresponse(); response.read(); c.close()
                if response.status == 200:
                    return
            except OSError:
                pass
            time.sleep(.25)
        self.fail('Owned app startup timeout')

    def stamped(self, name, fields, login, stamp):
        started = time.monotonic()
        result = shared.TalentScopeMySQLTests.stamped(self, name, fields, login, stamp)
        self.timings.append({'request': name, 'milliseconds': round((time.monotonic()-started)*1000, 3)})
        return result

    def test_large_bag_partial_breach_and_parallel_receipt(self):
        self.start()
        login = self.login('awake-load-' + secrets.token_hex(4))
        self.call('CSPlayerCreate', {'playerName': '觉醒负载', 'heroId': 'hero_043'}, login, 100, 'SCPlayerCreate')
        player, game = self.profile(login)
        player.update(level=70, coin=1000000, diamond=1000, warriorBagSize=1200, noviceGuideSchedule=8)
        base = dict(id='hero_003', level=1, exp=0, talent=0, training={}, tempTraining={}, breached={'level': 1}, awake={})
        game['warriors'] += [dict(base, guid='awake-load-%04d' % i) for i in range(1000)]
        game['warriors'].append(dict(base, guid='awake-load-target', level=70))
        game['items'] = [dict(id=k, num=n) for k,n in [('item_006',2),('item_322',800),('item_315',50),('item_312',15)]]
        game['kv'] = (game.get('kv') or []) + [dict(key='awake-load-unused-%04d' % i, value='x') for i in range(2000)]
        self.write_profile(login, player, game)
        self.call('CSWarriorBreached', {'guid': 'awake-load-0000', 'type': 1}, login, 101, 'SCWarriorBreached')
        _, game = self.profile(login)
        partial = next(w for w in game['warriors'] if w['guid']=='awake-load-0000')
        self.assertEqual(partial['breached']['exp'], 2)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies = list(pool.map(lambda _: self.call('CSWarriorAwake', {'guid':'awake-load-target'}, login, 102, 'SCWarriorAwake'), range(8)))
        self.assertTrue(all(r == replies[0] for r in replies))
        self.assertEqual({r.guid for r in replies[0].cost if r.guid}, {'awake-load-0001','awake-load-0002'})
        _, game = self.profile(login)
        self.assertEqual(next(w for w in game['warriors'] if w['guid']=='awake-load-0000'), partial)
        self.assertEqual(len(game['warriors']), 1000)
        self.stop(); self.start()
        self.assertEqual(self.call('CSWarriorAwake', {'guid':'awake-load-target'}, login, 102, 'SCWarriorAwake'), replies[0])
        samples = [r for r in self.timings if r['request']=='CSWarriorAwake']
        output = Path(os.environ['HYJS_AWAKE_AUDIT_OUTPUT']).with_name('large-bag-http.json')
        output.write_text(json.dumps(dict(status='PASS', fixture='isolated finite 1000 same-card donors, level70 target, 2000 inert KV entries; real partial breach',
            originalHTTP=True, parallelSameStamp=8, originalPartialBreachPreserved=True, coldReplay=True, timings=samples,
            binarySHA256=hashlib.sha256(Path(self.binary).read_bytes()).hexdigest()),indent=2)+'\n')
        for row in samples:
            self.assertLess(row['milliseconds'],1000,row)

    def test_all15_fragment_talent_awake_fusion100_refund_and_restart(self):
        if os.environ.get('HYJS_AWAKE_DOMAIN_SEPARATE') != '1':
            domain = subprocess.run(['rtk', 'proxy', os.environ['HYJS_AWAKE_AUDIT_TEST_BINARY'], '-test.run=^TestP5OriginalAwakeFullLifecycle$', '-test.count=1', '-test.timeout=180s'],
                                    cwd=shared.ROOT / 'hyjs-server/service', env=dict(self.env, HYJS_P5_TALENT_MYSQL_DATABASE=self.database), text=True, capture_output=True, timeout=190)
            Path(os.environ['HYJS_AWAKE_AUDIT_OUTPUT']).with_suffix('.domain.log').write_text(self.pool.redact(domain.stdout+domain.stderr))
            self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB complete awakening domain regression failed')
        self.start()
        heroes = [row for row in shared.original_table('Hero') if row['heroClass'] == '0' and row['IsOpenAwake'] == '1']
        fragments = {row['target']: row for row in shared.original_table('Frag')}
        awake_costs = shared.original_table('HeroAwakenCost')
        fuse_levels = shared.original_table('HeroAwakenFuseLevel')
        stamp = 1800000200000
        results = []
        restart = []
        for hero in heroes:
            hero_id = hero['id']
            uid = 'awake-' + hero_id + '-' + secrets.token_hex(3)
            login = self.login(uid)
            self.call('CSPlayerCreate', {'playerName': '原觉醒闭环', 'heroId': 'hero_043'}, login, stamp, 'SCPlayerCreate')
            player, game = self.profile(login)
            player.update(level=100, noviceGuideSchedule=8, coin=100000000, diamond=1000, warriorBagSize=1000)
            resources = {'item_005': 100000, 'item_312': 100000}
            for row in awake_costs:
                if row['heroId'] == hero_id:
                    for part in row['itemCost'].split(';'):
                        resources[part.split(',')[0]] = 100000
            fragment = fragments[hero_id]
            resources[fragment['id']] = int(fragment['num']) * 8
            game['items'] = [{'id': key, 'num': value} for key, value in resources.items()]
            self.write_profile(login, player, game)
            target = None
            for _ in range(8):
                stamp += 1
                gain = self.call('CSCombine', {'targetId': hero_id}, login, stamp, 'SCCombine')
                self.assertEqual(gain.get.wars[0].id, hero_id)
                target = target or gain.get.wars[0]
            player, game = self.profile(login)
            owned = next(row for row in game['warriors'] if row['guid'] == target.guid)
            owned['level'] = 100
            self.write_profile(login, player, game)
            initial = self.profile(login)
            stamp += 1
            fields = {'guid': target.guid, 'talent': '1', 'cost': [{'id': 'coin', 'num': 10000}, {'id': 'item_005', 'num': 30}]}
            self.call('CSWarriorTalent', fields, login, stamp, 'SCWarriorTalent')
            fields = {'guid': target.guid}
            for stage in (1, 2):
                stamp += 1
                if stage == 1:
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        replies = list(pool.map(lambda _: self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake'), range(8)))
                    self.assertTrue(all(reply == replies[0] for reply in replies))
                    response = replies[0]
                else:
                    response = self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake')
                self.assertEqual(response.newWarrior.awake.stage, stage)
                self.assertEqual(self.call('CSWarriorAwake', fields, login, stamp, 'SCWarriorAwake'), response)
            exp = sum(int(row['needExp']) for row in fuse_levels if row['quality'] == hero['quality'])
            number = (exp + 499) // 500
            stamp += 1
            fuse = {'guid': target.guid, 'cost': [{'id': 'item_312', 'num': number}]}
            fused = self.call('CSAwakeFuse', fuse, login, stamp, 'SCAwakeFuse')
            self.assertEqual((fused.newWarrior.awake.stage, fused.newWarrior.awake.fuseLevel, fused.newWarrior.awake.fuseExp), (2, 100, 0))
            self.assertEqual(self.call('CSAwakeFuse', fuse, login, stamp, 'SCAwakeFuse'), fused)
            stamp += 1
            ordered = hero['AwakeSkillId'] + ',' + hero['SkillId2'] + ',' + hero['SkillId1']
            self.call('CSSetSkillOrder', {'heroGuid': target.guid, 'skillOrder': ordered}, login, stamp, 'SCSetSkillOrder')
            before = self.profile(login)
            stamp += 1
            self.assertEqual(self.stamped('CSAwakeFuse', fuse, login, stamp)[0], 10001)
            self.assertEqual(self.stamped('CSWarriorAwake', fields, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            stamp += 1
            reset = self.call('CSWarriorReset', fields, login, stamp, 'SCWarriorReset')
            saved = self.profile(login)
            self.assertEqual(saved[0]['coin'], initial[0]['coin'] - 10000)
            self.assertEqual(saved[0]['diamond'], initial[0]['diamond'] - 50)
            self.assertEqual(len(saved[1]['warriors']), len(initial[1]['warriors']) - 8)
            self.assertFalse(any(w['guid'] == target.guid for w in saved[1]['warriors']))
            expected_items = {w['id']: w['num'] for w in initial[1]['items']}
            expected_items[fragment['id']] = expected_items.get(fragment['id'], 0) + int(fragment['num']) * 8
            actual_items = {w['id']: w['num'] for w in saved[1]['items']}
            self.assertEqual(actual_items, expected_items)
            reset_stamp = stamp
            stamp += 1
            self.call('CSWarriorAwake', fields, login, stamp, 'ErrorMessage')
            fresh = self.call('CSCombine', {'targetId': hero_id}, login, stamp + 1, 'SCCombine').get.wars[0]
            self.assertNotEqual(fresh.guid, target.guid)
            self.assertEqual((fresh.level, fresh.talent, fresh.awake.stage, fresh.awake.fuseLevel), (1, 0, 0, 0))
            self.call('CSFormationOn', {'formatPos': 2, 'id': hero_id, 'guid': fresh.guid}, login, stamp + 2, 'SCFormationOn')
            self.call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login, stamp + 3, 'SCCopyEnter')
            battle = self.call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login, stamp + 4, 'SCCopyRunMapInfo')
            actor = next(w for w in battle.battlePlayer.formation if w.guid == fresh.guid)
            self.assertEqual((actor.awakeStage, actor.awakenSkill, actor.skillOrder),
                             (0, '', hero['SkillId1'] + ',' + hero['SkillId2'] + ',' + hero['UltSkillId']))
            saved = self.profile(login)
            results.append({'heroId': hero_id, 'acquisition': 'eight original CSCombine requests from exact finite fragment fixtures',
                            'talent': 1, 'awakeStages': [1, 2], 'maxFuse': 100, 'fusionCost': number,
                            'concurrentAwake': 'eight identical receipts, one consumption', 'fullActualRefund': 'passed',
                            'highestAwakenedSkillReorderToResetToFormation': 'passed'})
            restart.append((uid, fresh.guid, fields, reset_stamp, reset, saved))
            stamp += 5
        self.stop()
        self.start()
        for uid, guid, fields, old_stamp, reset, saved in restart:
            login = self.login(uid)
            self.assertEqual(self.call('CSWarriorReset', fields, login, old_stamp, 'SCWarriorReset'), reset)
            self.assertEqual(self.profile(login), saved)
            warrior = next(w for w in self.call('CSWarriorList', {}, login, stamp + 1, 'SCWarriorList').warriorList if w.guid == guid)
            self.assertEqual((warrior.talent, warrior.awake.stage, warrior.awake.fuseLevel), (0, 0, 0))
        self.assertEqual(len(results), 15)
        output = Path(os.environ['HYJS_AWAKE_AUDIT_OUTPUT'])
        output.write_text(json.dumps({
            'date': '2026-10-26', 'allOriginalEnabledHeroes': results, 'originalHTTP': 'passed',
            'goInnoDBFullLifecycle': 'separate record' if os.environ.get('HYJS_AWAKE_DOMAIN_SEPARATE') == '1' else 'passed', 'processRestartReceipts': '15/15 retained without duplicate refund',
            'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(), 'sourceSha256': self.source_shas,
            'httpTimings': self.timings,
            'productionModified': False, 'remaining': ['natural fragment/material source per ID', 'phone UI', 'dynamic native awakening combat golden'],
            'preparation': 'finite original fragment/material balances and level prerequisite in dedicated test saves only'}, ensure_ascii=False, indent=2) + '\n')
        for sample in self.timings:
            if sample['request'] in {'CSWarriorAwake', 'CSAwakeFuse', 'CSWarriorList', 'CSSetSkillOrder'}:
                self.assertLess(sample['milliseconds'], 1000, sample)


if __name__ == '__main__':
    unittest.main()
