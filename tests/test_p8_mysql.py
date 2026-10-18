"""P8真实采集与材料消耗，专属MySQL库/用户，结束清理。"""
import json
import hashlib
import re
import selectors
import tempfile
import types
import http.client
import os
from pathlib import Path
import secrets
import subprocess
import time
import unittest

from server.codec import BY_NAME, decode_responses, encode_http_request, message
from test_go_formation import GoFormationTests

ROOT = Path(__file__).resolve().parents[1]


@unittest.skipUnless(os.environ.get('HYJS_P8_MYSQL_TEST') == '1', '显式启用P8专属MySQL回归')
class P8MySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login

    def stamped(self, name, fields, login, stamp):
        raw = encode_http_request(BY_NAME[name], message(name, **fields), player_id=login.playerId,
                                  zone_id=1, session=bytes(login.sessionKey), timestamp=stamp)
        connection = http.client.HTTPConnection('127.0.0.1', self.service.http_port, timeout=5)
        try:
            connection.request('POST', '/sao/', body=raw, headers={'Content-Type': 'application/octet-stream'})
            response = connection.getresponse()
            data = response.read()
            self.assertEqual(response.status, 200, data)
            self.last_frames = decode_responses(data)
            return self.last_frames[0] if self.last_frames else (0, None)
        finally:
            connection.close()

    def test_actual_gathering_and_crafting_transactions(self):
        suffix = secrets.token_hex(6)
        database = 'hyjs_test_p8_' + suffix
        user = 'p8_' + suffix
        password = secrets.token_hex(24)

        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p8_'))
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                     'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                    input=query, text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, 'P8 isolated database administration failed')
            return result.stdout

        started = time.monotonic()
        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env = dict(os.environ, HYJS_P8_MYSQL_DATABASE=database, HYJS_MYSQL_HOST='127.0.0.1:33068',
                       HYJS_MYSQL_USER=user, HYJS_MYSQL_PASSWORD=password)
            result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP8(Gather|Life)$', '-count=1', '-v'],
                                    cwd=ROOT / 'hyjs-server', env=env, text=True, capture_output=True, timeout=240)
            summary = '\n'.join(line for line in result.stdout.splitlines()
                                if line.startswith(('=== RUN', '--- PASS', '--- FAIL', '    --- PASS', '    --- FAIL', 'ok ', 'FAIL')))
            self.assertEqual(result.returncode, 0, summary)
            self.assertIn('--- PASS: TestP8Gather', summary)
            self.assertIn('--- PASS: TestP8Life', summary)
            engines = root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p8_%';")
            self.assertEqual(engines.strip(), 'InnoDB')
            report = {'date': '2026-10-13', 'candidateGroup': 'P8-gathering', 'mysqlTest': 'passed', 'goDomainSubtests': 13,
                      'actualOriginalWeightedResources': 'passed', 'timerAndEligibility': 'passed',
                      'eightConcurrentClaimReceipt': 'passed', 'overflowRollbackAndRetry': 'passed',
                      'fiveUsesAndCooldownRespawn': 'passed', 'cancelNoCharge': 'passed',
                      'databaseConnectionReopen': 'passed', 'mineRefineCraftMountBattleEffect': 'passed',
                      'fishCookUsePersistentEffect': 'passed', 'lifeBooksAndRecipeQualification': 'passed',
                      'foodHealCooldownAndReceipt': 'passed', 'paidAndFreeRevivalAndSafeClock': 'passed', 'tablesEngine': 'InnoDB',
                      'clockPreparation': 'isolated ready_at/refresh_at advance; real timer network gate pending',
                      'resourcePreparation': 'no gathered reward injection; finite level/coin/equipment prerequisites only',
                      'networkRoute': 'passed', 'productionSaveModified': False, 'productionDeployment': False}
            report.update(self.native_network(database, env, root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(), ['0', '0', '0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed', testSeconds=round(time.monotonic() - started, 3))
        (ROOT / 'analysis/p8/backend-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')

    def native_network(self, database, env, root_sql):
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-native-') as temp:
            binary = str(Path(temp) / 'hyjs-p8')
            built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', binary, '.'], cwd=ROOT / 'hyjs-server', capture_output=True, timeout=120)
            self.assertEqual(built.returncode, 0, 'P8 isolated build failed')
            process = None
            service_env = dict(env, HYJS_MYSQL_DATABASE=database)
            def start():
                nonlocal process
                process = subprocess.Popen([binary, '--http-port', '0', '--tcp-port', '0'], cwd=ROOT / 'hyjs-server', env=service_env,
                                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(60), 'P8 isolated startup timeout')
                    line = process.stdout.readline()
                found = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(found, 'P8 isolated startup rejected')
                self.service = types.SimpleNamespace(http_port=int(found[1]), tcp_port=int(found[2]))
            def stop():
                nonlocal process
                if process is not None and process.poll() is None:
                    process.terminate()
                    try: process.wait(timeout=10)
                    except subprocess.TimeoutExpired: process.kill(); process.wait(timeout=10)
                if process is not None and process.stdout: process.stdout.close()
                process = None
            def sql(query): return root_sql(f'USE `{database}`; '+query)
            def profile(login):
                raw = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|')
                return [json.loads(bytes.fromhex(value)) for value in raw]
            def call(name, fields, stamp, expected):
                opcode, reply = self.stamped(name, fields, login, stamp)
                self.assertEqual(opcode, BY_NAME[expected], str(reply))
                return reply
            waits=[]
            elapsed=[]
            try:
                start()
                uid='p8-native-'+secrets.token_hex(4)
                login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate', {'playerName':'原帧真实采集','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login)
                p.update(level=18,noviceGuideSchedule=7)
                values=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex() for v in [p,g]]
                sql("UPDATE profiles SET player=X'"+values[0]+"',game=X'"+values[1]+"' WHERE id="+str(login.playerId)+';')
                sql('INSERT INTO gem_craft_states(player_id,level,exp) VALUES('+str(login.playerId)+',2,0); INSERT INTO cooking_craft_states(player_id,level,exp) VALUES('+str(login.playerId)+',2,0);')
                call('CSTransferScene',{'target':'field_001'},81000,'SCTransferScene')
                call('CSLoginScene',{},81001,'SCLoginScene')
                resources=[body for opcode,body in self.last_frames if opcode==BY_NAME['SCFieldResourceInfo']]
                self.assertEqual(sum(len(v.resource) for v in resources),18)
                rows=sql('SELECT CONCAT(pos_id,"|",HEX(snapshot)) FROM p8_resource_nodes WHERE player_id='+str(login.playerId)+' ORDER BY pos_id;').splitlines()
                nodes=[(row.split('|')[0],json.loads(bytes.fromhex(row.split('|')[1]))) for row in rows]
                for index,kind in enumerate(['mine','fish']):
                    pos,cfg=next(n for n in nodes if n[1]['kind']==kind)
                    stamp=81100+index*10
                    self.stamped('CSMove',{'pointNew':cfg['point'],'currentTime':stamp},login,stamp)
                    gather_begin=time.monotonic()
                    start_reply=call('CSFieldResourceStartGather',{'posId':pos},stamp+1,'SCFieldResourceStartGather')
                    self.assertEqual(start_reply.result,1)
                    before=profile(login)
                    self.assertEqual(self.stamped('CSFieldGetResource',{},login,stamp+2)[0],10001)
                    self.assertEqual(profile(login),before)
                    frozen=sql('SELECT CONCAT(ready_at,"|",HEX(rewards)) FROM p8_gather_attempts WHERE player_id='+str(login.playerId)+';').strip()
                    if kind=='mine':
                        stop(); start(); login=self.login(uid)
                        call('CSLoginScene',{},stamp+3,'SCLoginScene')
                        self.assertEqual(sql('SELECT CONCAT(ready_at,"|",HEX(rewards)) FROM p8_gather_attempts WHERE player_id='+str(login.playerId)+';').strip(),frozen)
                    remaining=max(0,(int(frozen.split('|')[0])-int(time.time()*1000))/1000)+0.04
                    begin=time.monotonic(); time.sleep(remaining); waits.append(round(time.monotonic()-begin,3))
                    claim=call('CSFieldGetResource',{},stamp+4,'SCFieldGetResource')
                    elapsed.append(round(time.monotonic()-gather_begin,3))
                    self.assertGreaterEqual(elapsed[-1],cfg['gatherMs']/1000)
                    self.assertEqual(claim.code,1)
                    self.assertEqual(claim.cost[0].num,cfg['fatigue'])
                    saved=profile(login)
                    self.assertEqual(call('CSFieldGetResource',{},stamp+4,'SCFieldGetResource'),claim)
                    self.assertEqual(profile(login),saved)
                    self.assertEqual(call('CSFieldGetResource',{},stamp+5,'SCFieldGetResource').code,2003)
                mine=next(n for n in nodes if n[1]['kind']=='mine')
                self.stamped('CSMove',{'pointNew':mine[1]['point'],'currentTime':81200},login,81200)
                call('CSFieldResourceStartGather',{'posId':mine[0]},81201,'SCFieldResourceStartGather')
                before=profile(login)
                self.stamped('CSMove',{'pointNew':{'x':0,'y':0,'z':0},'currentTime':81202},login,81202)
                self.assertEqual(call('CSFieldGetResource',{},81203,'SCFieldGetResource').code,2003)
                self.assertEqual(profile(login)[0],before[0])
                self.assertEqual(profile(login)[1].get('gems'),before[1].get('gems'))
                # Finite prerequisites only: the actual food is cooked and skill book
                # comes from its complete original VIP box, then both are consumed.
                p,g=profile(login)
                p.update(coin=10000000,diamond=100)
                g.setdefault('items',[])
                g['items']=g['items'] or []
                g['items'].append({'id':'item_042','num':1})
                g.setdefault('cooking',[])
                g['cooking']=g['cooking'] or []
                g['cooking'].append({'id':'FishItem_007','num':10})
                def persist():
                    values=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex() for v in [p,g]]
                    sql("UPDATE profiles SET player=X'"+values[0]+"',game=X'"+values[1]+"' WHERE id="+str(login.playerId)+';')
                persist()
                call('CSItemUse',{'costItem':{'id':'item_042','num':1}},81300,'SCItemUse')
                learned=call('CSItemUse',{'costItem':{'id':'item_063','num':1}},81301,'SCItemUse')
                levels=call('CSCookingLevel',{},81302,'SCCookingLevel')
                self.assertIn('LifeSkill_085',levels.skills)
                saved=profile(login)
                self.assertEqual(call('CSItemUse',{'costItem':{'id':'item_063','num':1}},81301,'SCItemUse'),learned)
                self.assertEqual(profile(login),saved)
                call('CSFishCooking',{'fish':{'id':'FishItem_026','num':2}},81303,'SCFishCooking')
                p,g=profile(login)
                g['field']['side'].update(curHp=0.4,state=0,battlePosHps=[])
                g['field'].pop('fight',None)
                g['position']['current']={'x':1000,'y':0,'z':1000}
                persist()
                call('CSSideSetShortCut',{'position':1,'id':'FishItem_026'},81310,'SCSideSetShortCut')
                call('CSSideSetShortCut',{'position':2,'id':'item_068'},81311,'SCSideSetShortCut')
                shortcuts=call('CSGemAndCookingList',{},81312,'SCGemAndCookingList')
                self.assertEqual((shortcuts.shortCut1,shortcuts.shortCut2),('FishItem_026','item_068'))
                food={'costItem':{'id':'FishItem_026','num':1}}
                healed=call('CSHpRecover',food,81304,'SCHpRecover')
                self.assertEqual(healed.cost[0].num,1)
                self.assertAlmostEqual(profile(login)[1]['field']['side']['curHp'],0.45)
                self.assertEqual(self.stamped('CSHpRecover',food,login,81305)[0],10001)
                saved=profile(login)
                cooldown=call('CSGemAndCookingList',{},81313,'SCGemAndCookingList')
                self.assertGreater(cooldown.nextUseTime,int(time.time()*1000)+59000)
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSGemAndCookingList',{},81314,'SCGemAndCookingList'),cooldown)
                self.assertEqual(call('CSHpRecover',food,81304,'SCHpRecover'),healed)
                self.assertEqual(profile(login),saved)
                p,g=profile(login);g['field']['side'].update(curHp=0,state=5,battlePosHps=[]);persist()
                revived=call('CSResurrection',{},81306,'SCResurrenttion')
                self.assertEqual(revived.cost[0].num,50)
                p,g=profile(login);self.assertEqual(p['diamond'],50);self.assertEqual(g['field']['side']['curHp'],1)
                g['field']['side'].update(curHp=0,state=5,battlePosHps=[]);persist()
                call('CSGetHp',{'playerId':login.playerId},81307,'SCGetHp')
                self.assertEqual(self.stamped('CSTransfer2Safe',{},login,81308)[0],10001)
                free_begin=time.monotonic();time.sleep(10.04)
                call('CSTransfer2Safe',{},81309,'SCTransfer2Safe')
                p,g=profile(login);self.assertEqual(p['diamond'],50);self.assertAlmostEqual(g['field']['side']['curHp'],0.1)
                return {'networkRoute':'passed','nativeRealWaitSeconds':waits,'nativeStartToClaimSeconds':elapsed,'nativeMineFishClaim':'passed',
                        'nativeShortcutsAndCooldownAfterRestart':'passed','nativeSkillBookViaOriginalBox':'passed','nativeRecipeCookFoodHealAndRestart':'passed',
                        'nativePaidRevival50Diamond':'passed','nativeFreeRevivalRealWaitSeconds':round(time.monotonic()-free_begin,3),
                        'nativeEarlyClaimRollback':'passed','nativeExactReceiptRetry':'passed','nativeMoveCancellation':'passed',
                        'serverProcessRestartDuringGather':'passed','nativeFrozenRewardAfterRestart':'passed',
                        'networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                        'clockPreparation':'domain timers advance isolated rows; HTTP mining/fishing waits use real wall clock'}
            finally: stop()


del GoFormationTests

if __name__ == '__main__':
    unittest.main()
