"""All original talent rows on disposable InnoDB and original binary HTTP frames."""
import concurrent.futures
import csv
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import shutil
import subprocess
import tempfile
import types
import unittest

from server.codec import BY_NAME
import test_p10_mysql as transport

ROOT = Path(__file__).resolve().parents[1]


def original_table(name):
    with (ROOT / 'analysis/tables' / ('Table' + name + '.tsv')).open() as handle:
        rows = list(csv.reader(handle, delimiter='\t'))
    return [dict(zip(rows[1], row)) for row in rows[5:] if row and row[0]]


@unittest.skipUnless(os.environ.get('HYJS_P5_TALENT_MYSQL_TEST') == '1', 'Explicit isolated full talent MySQL test')
class TalentScopeMySQLTests(unittest.TestCase):
    send = transport.P10MySQLTests.send
    login = transport.P10MySQLTests.login
    stamped = transport.P10MySQLTests.stamped
    call = transport.P10MySQLTests.call
    profile = transport.P10MySQLTests.profile
    start = transport.P10MySQLTests.start
    stop = transport.P10MySQLTests.stop
    setUp = transport.P10MySQLTests.setUp

    @classmethod
    def setUpClass(cls):
        cls.database = 'hyjs_test_p5_talent_' + secrets.token_hex(6)
        cls.test_user = 'p5talent_' + secrets.token_hex(5)
        cls.test_password = secrets.token_hex(24)
        cls.temp = tempfile.TemporaryDirectory(prefix='hyjs-p5-talent-')
        cls.addClassCleanup(cls.temp.cleanup)
        cls.binary = str(Path(cls.temp.name) / 'hyjs-talent')
        cls.root_sql(f"CREATE DATABASE `{cls.database}` CHARACTER SET utf8mb4; CREATE USER '{cls.test_user}'@'%' IDENTIFIED BY '{cls.test_password}'; GRANT ALL ON `{cls.database}`.* TO '{cls.test_user}'@'%';")

        def cleanup():
            cls.root_sql(f"DROP DATABASE IF EXISTS `{cls.database}`; DROP USER IF EXISTS '{cls.test_user}'@'%';")
            left = cls.root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{cls.database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{cls.test_user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{cls.database}';")
            if left.split() != ['0', '0', '0']:
                raise AssertionError('Full talent test schema/user/grants remain')
        cls.addClassCleanup(cleanup)
        cls.env = dict(os.environ, HYJS_MYSQL_HOST='127.0.0.1:33068', HYJS_MYSQL_USER=cls.test_user,
                       HYJS_MYSQL_PASSWORD=cls.test_password, HYJS_MYSQL_DATABASE=cls.database,
                       HYJS_P5_TALENT_MYSQL_DATABASE=cls.database)
        # Freeze build input while the other authorized agents continue editing.
        source = Path(cls.temp.name) / 'source'
        base = Path(os.environ.get('HYJS_P5_BASE_SOURCE', ROOT / 'hyjs-server')).resolve()
        if base != (ROOT / 'hyjs-server').resolve() and (ROOT / 'build/candidates').resolve() not in base.parents:
            raise AssertionError('Growth baseline must be a local fixed candidate')
        shutil.copytree(base, source, ignore=shutil.ignore_patterns('logs'))
        if base != (ROOT / 'hyjs-server').resolve():
            # Independent new growth changes overlay a previously verified joint
            # baseline without reading other agents' unfinished shared edits.
            for relative in ('service/treasureService.go', 'service/p5RecycleService.go',
                             'service/p5TreasureFullScope_test.go', 'service/p5TreasureRefineFullScope_test.go',
                             'service/p5EquipmentHighestFullScope_test.go',
                             'service/equipmentRecycleService.go',
                             'service/p5FormationEligibilityService.go', 'service/p5FormationEligibility_test.go',
                             'constUtil/p5FormationEligibility.go',
                             'service/p5GrowthReceiptService.go', 'constUtil/p5GrowthReceipt.go',
                             'service/warriorGrowthService.go', 'service/equipmentAdvanceService.go',
                             'service/equipmentService.go', 'constUtil/equipmentMutation.go',
                             'service/p5Service.go', 'service/skillSpecialtyService.go',
                             'service/p5TalentFullScope_test.go'):
                shutil.copy2(ROOT / 'hyjs-server' / relative, source / relative)
        if os.environ.get('HYJS_P5_MUTATION_RETRY_SOURCE') == '1':
            for relative in ('main.go', 'service/gameService.go', 'service/profileService.go', 'service/copyService.go', 'service/formationService.go', 'service/paymentService.go', 'service/eventService.go', 'service/p4ActivityService.go', 'constUtil/serverIdentity.go'):
                shutil.copy2(ROOT / 'hyjs-server' / relative, source / relative)
        cls.baseline_source = str(base.relative_to(ROOT))
        cls.frozen_source = source
        (Path(cls.temp.name) / 'analysis').symlink_to(ROOT / 'analysis', target_is_directory=True)
        cls.source_shas = {str(path.relative_to(source)): hashlib.sha256(path.read_bytes()).hexdigest()
                           for path in sorted(source.rglob('*.go'))}
        built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', cls.binary, '.'],
                               cwd=source, capture_output=True, timeout=180)
        if built.returncode:
            raise AssertionError('Full talent candidate build failed: '+(built.stdout+built.stderr).decode(errors='replace')[-4000:])

    def start(self):
        args = [self.binary, '--http-port', '0', '--tcp-port', '0', '--tables', str(ROOT / 'analysis/tables')]
        if 'flag.String("log-file"' in (self.frozen_source / 'main.go').read_text():
            args.extend(['--log-file', str(Path(self.temp.name) / 'app.log')])
        self.process = subprocess.Popen(args, cwd=ROOT / 'hyjs-server', env=self.env,
                                        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
        with selectors.DefaultSelector() as ready:
            ready.register(self.process.stdout, selectors.EVENT_READ)
            self.assertTrue(ready.select(90), 'Growth isolated service startup timeout')
            line = self.process.stdout.readline()
        ports = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
        self.assertIsNotNone(ports, line)
        self.service = types.SimpleNamespace(http_port=int(ports[1]), tcp_port=int(ports[2]))

    @classmethod
    def root_sql(cls, text):
        if not cls.database.startswith('hyjs_test_p5_talent_') or not cls.test_user.startswith('p5talent_'):
            raise AssertionError('Full talent isolation guard')
        result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                 'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                input=text, text=True, capture_output=True, timeout=30)
        if result.returncode:
            raise AssertionError('Full talent isolated database administration failed')
        return result.stdout

    def sql(self, text):
        return self.root_sql('USE `' + self.database + '`; ' + text)

    def write_profile(self, login, player, game):
        raw = [json.dumps(value, ensure_ascii=False, separators=(',', ':')).encode().hex() for value in (player, game)]
        self.sql(f"UPDATE profiles SET player=X'{raw[0]}',game=X'{raw[1]}' WHERE id={login.playerId};")

    def test_all_1003_original_rows_and_74_partner_projection(self):
        domain = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP5(OriginalTalentFullScope|OriginalGreenPartnerProjection|AllOriginalPartnerSkillSlots)$', '-count=1'],
                                cwd=ROOT / 'hyjs-server', env=self.env, text=True, capture_output=True, timeout=180)
        self.assertEqual(domain.returncode, 0, 'Dedicated InnoDB full talent domain regression failed')
        self.start()
        heroes = {row['id']: row for row in original_table('Hero')}
        nodes = {}
        for row in original_table('TalentAdvenced'):
            nodes.setdefault(row['warId'], []).append(row)
        self.assertEqual(sum(map(len, nodes.values())), 1003)
        self.assertEqual(len(nodes), 76)
        results = []
        stamp = 1800000100000
        restart = []
        prepared = []
        # Original account creation has shared initial grant/event rows. Prepare
        # these prerequisites serially, then run independent growth concurrently.
        for index, (hero_id, rows) in enumerate(sorted(nodes.items())):
            login = self.login('p5-native-talent-' + hero_id)
            self.call('CSPlayerCreate', {'playerName': '原天赋全量', 'heroId': 'hero_043'}, login, stamp, 'SCPlayerCreate')
            player, game = self.profile(login)
            prepared.append((index, hero_id, rows, login, player, game))
        def exercise_hero(entry):
            index, hero_id, rows, login, player, game = entry
            stamp = 1800000100000 + index * 10000
            uid = 'p5-native-talent-' + hero_id
            player.update(level=100, coin=100000000, warriorBagSize=1000, noviceGuideSchedule=8)
            target = {'id': hero_id, 'guid': hero_id + '-native-target', 'level': 100, 'exp': 0, 'talent': 0,
                      'training': {}, 'tempTraining': {}, 'breached': {'level': 1}, 'awake': {}}
            game['warriors'] = [target]
            game['formation'] = [{'formatPos': 1, 'battlePos': 1,
                                  'things': [{'pos': 0, 'id': hero_id, 'guid': target['guid']}]}]
            game['items'] = [{'id': key, 'num': 100000} for key in ('item_005', 'item_047', 'item_470')]
            if heroes[hero_id]['heroClass'] != '0':
                player['heroWarId'] = target['guid']
            cost_totals = {}
            donors = {}
            for node in rows:
                for part in filter(None, node['cost'].split(';')):
                    resource, number = part.split(',')
                    cost_totals[resource] = cost_totals.get(resource, 0) + int(number)
            for resource, count in cost_totals.items():
                if resource.startswith('hero_'):
                    donors[resource] = []
                    for index in range(count):
                        donor = {'id': resource, 'guid': f'{hero_id}-material-{index}', 'level': 1, 'exp': 0, 'talent': 0,
                                 'training': {}, 'tempTraining': {}, 'breached': {'level': 1}, 'awake': {}}
                        game['warriors'].append(donor)
                        donors[resource].append(donor['guid'])
            # These are finite isolated fixtures. No production save is read or
            # edited, and fixture balances do not certify natural material supply.
            self.write_profile(login, player, game)
            initial = self.profile(login)
            actual = 0
            max_trunk = 0
            branch_ids = []
            last_fields = None
            for node in rows:
                if node['talentLevel'] == '0':
                    continue
                costs = []
                for part in node['cost'].split(';'):
                    resource, number = part.split(',')
                    number = int(number)
                    if resource.startswith('hero_'):
                        for _ in range(number):
                            costs.append({'id': hero_id, 'guid': donors[resource].pop(0)})
                    else:
                        costs.append({'id': resource, 'num': number})
                fields = {'guid': target['guid'], 'talent': node['talentLevel'], 'cost': costs}
                stamp += 1
                # Concurrent original requests must produce a single mutation.
                if hero_id == 'hero_001' and node['talentLevel'] == '1':
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                        replies = list(pool.map(lambda _: self.stamped('CSWarriorTalent', fields, login, stamp), range(8)))
                    success = [reply for opcode, reply in replies if opcode == BY_NAME['SCWarriorTalent']]
                    self.assertEqual(len(success), 1)
                    self.assertEqual(sum(opcode == 10001 for opcode, _ in replies), 7)
                    response = success[0]
                else:
                    response = self.call('CSWarriorTalent', fields, login, stamp, 'SCWarriorTalent')
                self.assertEqual(response.guid.id, hero_id)
                self.assertEqual(response.guid.guid, target['guid'])
                self.assertEqual(self.stamped('CSWarriorTalent', fields, login, stamp)[0], 10001)
                if node['trunck'] == '1':
                    max_trunk = max(max_trunk, int(node['talentLevel']))
                    self.assertEqual(response.guid.talent, int(node['talentLevel']))
                else:
                    branch_ids.append(node['talentLevel'])
                    self.assertIn(node['talentLevel'], response.guid.talentBranch)
                actual += 1
                last_fields = fields
            saved = self.profile(login)
            self.assertEqual(initial[0]['coin'] - saved[0]['coin'], cost_totals.get('coin', 0))
            items = {row['id']: row['num'] for row in saved[1]['items']}
            for resource in ('item_005', 'item_047', 'item_470'):
                self.assertEqual(100000 - items[resource], cost_totals.get(resource, 0))
            self.assertEqual(len(saved[1]['warriors']), 1)
            before = self.profile(login)
            stamp += 1
            self.assertEqual(self.stamped('CSWarriorTalent', {'guid': target['guid'], 'talent': str(max_trunk + 1)}, login, stamp)[0], 10001)
            self.assertEqual(self.profile(login), before)
            self.call('CSCopyEnter', {'chapterId': '1', 'missionId': '1'}, login, stamp, 'SCCopyEnter')
            battle = self.call('CSCopyRunMapInfo', {'chapterId': '1', 'missionId': '1'}, login, stamp, 'SCCopyRunMapInfo')
            actor = next(w for w in battle.battlePlayer.formation if w.guid == target['guid'])
            expected_talents = [row['talentId'] for row in rows if row['talentId']]
            self.assertEqual(list(actor.talentList), expected_talents)
            self.assertGreater(actor.prop.att, 0)
            self.assertGreater(actor.prop.hp, 0)
            if heroes[hero_id]['quality'] == '13' and heroes[hero_id]['heroClass'] == '0':
                self.assertEqual(actor.skillOrder, heroes[hero_id]['SkillId1'] + ',' + heroes[hero_id]['SkillId2'] + ',')
            results.append({'heroId': hero_id, 'zeroBaseline': True, 'actualActivations': actual,
                            'highestTrunk': max_trunk, 'branches': branch_ids, 'exactCosts': cost_totals,
                            'originalHTTP': 'passed', 'highestOverflowRollback': 'passed', 'battleProjection': 'passed'})
            restart.append((uid, target['guid'], max_trunk, branch_ids, last_fields))
        # Accounts and activation paths are independent; bounded workers keep
        # all original HTTP assertions while reducing serialized network cost.
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
            list(pool.map(exercise_hero, prepared))
        results.sort(key=lambda row: row['heroId'])
        restart.sort(key=lambda row: row[0])
        self.stop()
        self.start()
        for uid, guid, trunk, branches, fields in restart:
            login = self.login(uid)
            warrior = next(w for w in self.call('CSWarriorList', {}, login, stamp, 'SCWarriorList').warriorList if w.guid == guid)
            self.assertEqual((warrior.talent, list(warrior.talentBranch)), (trunk, branches))
            self.assertEqual(self.stamped('CSWarriorTalent', fields, login, stamp + 1)[0], 10001)
        self.assertEqual(sum(row['actualActivations'] for row in results), 927)
        report = {'date': '2026-10-14', 'originalRows': 1003, 'zeroBaselines': 76, 'actualActivations': 927,
                  'characterPaths': results, 'isolatedMySQL': True, 'originalHTTP': True,
                  'eightConcurrentActivation': 'one mutation, seven original errors',
                  'processRestart': 'all 76 paths retained', 'binarySha256': hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),
                  'sourceSha256': self.source_shas, 'productionModified': False,
                  'preparation': 'finite test material balances and zero-stage targets; source acquisition and native dynamic golden are independent unfinished tasks',
                  'remaining': ['natural materials for all IDs', 'phone UI', 'original dynamic combat golden', 'green no-skill battle player']}
        (ROOT / 'analysis/p5/talent-full-scope-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')


if __name__ == '__main__':
    unittest.main()
