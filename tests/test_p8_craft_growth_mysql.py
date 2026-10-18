"""P9 committed copy intimacy, focused isolated MySQL and native HTTP verification."""
import csv
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import selectors
import subprocess
import tempfile
import time
import types
import unittest
from server.codec import BY_NAME,message,encode_http_request
from test_go_formation import GoFormationTests

ROOT = Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P8_MYSQL_TEST') == '1', '显式启用P9MySQL回归')
class P8CraftGrowthMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_permanent_cooking_investment_refund_restart(self):
        suffix = secrets.token_hex(6)
        database = 'hyjs_test_p8_' + suffix
        user = 'p8_' + suffix
        password = secrets.token_hex(24)
        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p8_'))
            result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c',
                'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,'P9 isolated DB administration failed')
            return result.stdout
        began=time.monotonic()
        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env=dict(os.environ,HYJS_P8_MYSQL_DATABASE=database,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=user,HYJS_MYSQL_PASSWORD=password)
            result=subprocess.run(['rtk','proxy','go','test','./service','-run',
                '^TestP8CraftGrowth$','-count=1','-v'],
                cwd=ROOT/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(password,'<redacted>'))
            self.assertIn('--- PASS: TestP8CraftGrowth',result.stdout)
            cases=[v.split('=== RUN   ',1)[1]for v in result.stdout.splitlines()if v.startswith('=== RUN   ')and '/'in v]
            self.assertEqual(len(cases),3)
            engines=root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p8_%';")
            self.assertEqual(engines.strip(),'InnoDB')
            report={'date':'2026-10-13','candidateGroup':'P8-P5-permanent-cooking-growth','mysqlTest':'passed',
                'goDomainSubtests':3,'goDomainNames':cases,'tablesEngine':'InnoDB',
                'actualOriginalFirstMapFishToCraftToUseToHalfRefund':'passed','concurrentDishAndUntrackedGrowthRollback':'passed',
                'originalGemMountUnmountStoredPartyPower':'passed','productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p8/craft-growth-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


    def native_network(self,database,env,root_sql):
        import concurrent.futures
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-craft-growth-')as temp:
            binary=str(Path(temp)/'hyjs-p8-craft-growth')
            build=subprocess.run(['rtk','proxy','go','build','-o',binary,'.'],cwd=ROOT/'hyjs-server',capture_output=True,timeout=120)
            self.assertEqual(build.returncode,0,'P9 isolated build failed')
            process=None
            service_env=dict(env,HYJS_MYSQL_DATABASE=database)
            def start():
                nonlocal process
                process=subprocess.Popen([binary,'--http-port','0','--tcp-port','0'],cwd=ROOT/'hyjs-server',env=service_env,
                    stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
                with selectors.DefaultSelector()as ready:
                    ready.register(process.stdout,selectors.EVENT_READ)
                    self.assertTrue(ready.select(90),'P9 isolated startup timeout')
                    line=process.stdout.readline()
                found=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
                self.assertIsNotNone(found,'P9 isolated startup rejected')
                self.service=types.SimpleNamespace(http_port=int(found[1]),tcp_port=int(found[2]))
            def stop():
                nonlocal process
                if process is not None and process.poll()is None:
                    process.terminate()
                    try:process.wait(timeout=10)
                    except subprocess.TimeoutExpired:process.kill();process.wait(timeout=10)
                if process is not None and process.stdout:process.stdout.close()
                process=None
            def sql(query):return root_sql(f'USE `{database}`; '+query)
            def profile(login):
                raw=sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id='+str(login.playerId)+';').strip().split('|')
                return [json.loads(bytes.fromhex(v))for v in raw]
            def persist(login,p,g):
                raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[p,g]]
                sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(login.playerId)+';')
            def call(name,fields,login,stamp,expected):
                opcode,result=self.stamped(name,fields,login,stamp)
                self.assertEqual(opcode,BY_NAME[expected],str(result));return result

            def move(point,stamp):
                import http.client
                raw=encode_http_request(BY_NAME['CSMove'],message('CSMove',pointNew=point,currentTime=stamp),player_id=login.playerId,zone_id=1,session=bytes(login.sessionKey),timestamp=stamp)
                connection=http.client.HTTPConnection('127.0.0.1',self.service.http_port,timeout=5)
                try:
                    connection.request('POST','/sao/',body=raw,headers={'Content-Type':'application/octet-stream'})
                    response=connection.getresponse();response.read();self.assertEqual(response.status,200)
                finally:connection.close()
                self.assertEqual(profile(login)[1]['position']['current'],point)
            try:
                start();uid='p8-craft-native-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原永久料理闭环','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=20,noviceGuideSchedule=7,coin=1000000,diamond=200)
                g['items']=[{'id':'item_053','num':1}];persist(login,p,g)
                box=call('CSItemUse',{'costItem':{'id':'item_053','num':1},'getItemIndex':2},login,96000,'SCItemUse')
                self.assertEqual([v.id for v in box.get.wars],['hero_003']);hero=box.get.wars[0]
                call('CSTransferScene',{'target':'field_001'},login,96001,'SCTransferScene')
                call('CSLoginScene',{},login,96002,'SCLoginScene')
                def count(key):return sum(v['num']for v in (profile(login)[1].get('cooking')or[])if v['id']==key)
                first_wait=None;claims=0
                for attempt in range(100):
                    if all(count(key)>=15 for key in ['FishItem_007','FishItem_013','FishItem_019']):break
                    raw=sql('SELECT CONCAT(pos_id,"|",HEX(snapshot),"|",remaining) FROM p8_resource_nodes WHERE player_id='+str(login.playerId)+' ORDER BY pos_id;')
                    nodes=[(v.split('|')[0],json.loads(bytes.fromhex(v.split('|')[1])),int(v.split('|')[2]))for v in raw.splitlines()]
                    candidates=[v for v in nodes if v[1]['kind']=='fish'and v[1]['level']==1 and v[2]>0]
                    stamp=96100+attempt*4
                    if not candidates:
                        sql('UPDATE p8_resource_nodes SET refresh_at=1 WHERE player_id='+str(login.playerId)+' AND remaining=0;')
                        call('CSPingTcp',{'clientTime':stamp},login,stamp,'SCPingTcp')
                        continue
                    pos,cfg,_=candidates[0]
                    move(cfg['point'],stamp)
                    started=call('CSFieldResourceStartGather',{'posId':pos},login,stamp+1,'SCFieldResourceStartGather')
                    self.assertEqual(started.result,1)
                    frozen=sql('SELECT CONCAT(started_at,"|",ready_at,"|",HEX(rewards)) FROM p8_gather_attempts WHERE player_id='+str(login.playerId)+';').strip().split('|')
                    self.assertEqual(int(frozen[1])-int(frozen[0]),5000)
                    if first_wait is None:
                        began=time.monotonic();time.sleep(5.1);first_wait=round(time.monotonic()-began,3)
                    else:
                        # Only this disposable repeated-source test advances the wait.
                        # Rewards are already frozen and no inventory is injected.
                        sql('UPDATE p8_gather_attempts SET ready_at=1 WHERE player_id='+str(login.playerId)+';')
                    collected=call('CSFieldGetResource',{},login,stamp+2,'SCFieldGetResource')
                    self.assertEqual(collected.code,1)
                    self.assertEqual([(v.id,v.num)for v in collected.cost],[('tiredness',2)])
                    frozen_pairs=[v.split(',')for v in bytes.fromhex(frozen[2]).decode().split(';')]
                    self.assertEqual(sorted((v.id,v.num)for v in collected.res.simps),sorted((key,int(n))for key,n in frozen_pairs))
                    claims+=1
                self.assertTrue(all(count(key)>=15 for key in ['FishItem_007','FishItem_013','FishItem_019']),'finite original fish source must cover the original recipe')
                self.assertEqual(profile(login)[1]['field']['side']['tiredness'],200-claims*2)
                made=call('CSFishCooking',{'fish':{'id':'FishItem_029','num':3}},login,96600,'SCFishCooking')
                self.assertEqual(made.add,75);self.assertEqual(count('FishItem_029'),3)
                self.assertEqual(profile(login)[0]['coin'],940000)
                on=call('CSFormationOn',{'formatPos':2,'id':hero.id,'guid':hero.guid},login,96601,'SCFormationOn')
                food_request={'heroGuid':hero.guid,'costItem':{'id':'FishItem_029','num':1}}
                used=None
                for i in range(3):
                    used=call('CSFishUsing',food_request,login,96610+i,'SCFishUsing')
                    self.assertEqual(used.warrior.cooking.prop.att,i+1)
                self.assertEqual(count('FishItem_029'),0)
                self.assertEqual(call('CSFishUsing',food_request,login,96612,'SCFishUsing'),used)
                self.assertEqual(count('FishItem_029'),0)
                p,g=profile(login)
                ledger=json.loads(next(v['value']for v in g['kv']if v['key']=='p5.invest:'+hero.guid))
                self.assertEqual([(v['id'],v['num'])for v in ledger['costs']],[('FishItem_029',3)])
                before=profile(login)
                self.assertEqual(self.stamped('CSWarriorReset',{'guid':p['heroWarId']},login,96613)[0],10001)
                self.assertEqual(profile(login),before)
                call('CSFormationOff',{'formatPos':2,'id':hero.id,'guid':hero.guid},login,96620,'SCFormationOff')
                reset_request={'guid':hero.guid}
                reset=call('CSWarriorReset',reset_request,login,96621,'SCWarriorReset')
                self.assertEqual([(v.id,v.num)for v in reset.get.simps],[('FishItem_029',1)])
                self.assertEqual(count('FishItem_029'),1)
                self.assertFalse(reset.warriors[0].HasField('cooking'))
                self.assertEqual(profile(login)[0]['diamond'],150)
                self.assertEqual(call('CSWarriorReset',reset_request,login,96621,'SCWarriorReset'),reset)
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSWarriorReset',reset_request,login,96621,'SCWarriorReset'),reset)
                self.assertEqual(call('CSFishUsing',food_request,login,96612,'SCFishUsing'),used)
                self.assertEqual(count('FishItem_029'),1)
                reuse=call('CSFishUsing',food_request,login,96700,'SCFishUsing')
                self.assertEqual(reuse.warrior.cooking.prop.att,1)
                zero=call('CSWarriorReset',reset_request,login,96701,'SCWarriorReset')
                self.assertFalse(zero.get.simps);self.assertEqual(count('FishItem_029'),0)
                self.assertEqual(profile(login)[0]['diamond'],100)
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'actualOriginalFirstMapGatherClaims':claims,'firstRealFishWaitSeconds':first_wait,
                    'nativeRedSelectionBoxToUniqueAsuna':'passed','nativeFishFrozenRewardsCostsAndRecipe':'passed',
                    'nativePermanentFoodThreeAttAndActualInvestment':'passed','nativeOriginalHalfRefundThreeToOneAndReuseOneToZero':'passed',
                    'mainGUIDResetProtection':'passed','serverProcessRestart':'passed','nativeUseResetHistoricalReceipt':'passed',
                    'productionSourceStatus':'fixed525 formal fbf8f8f9 has no new craft-growth group; new code not deployed/native-accepted',
                    'originalRule':'TableText.recycle_desc_text returns50% dishes; integer odd cumulative units floor is explicit compatibility policy',
                    'originalPermanentGainBoundary':'existing CookingRule SQL increments are compatibility values, actual costs and props frozen; no new original gain claim',
                    'clockPreparation':'first HTTP fish uses real5.1s wait; repeated finite test-source waits advance only isolated ready_at/empty-node refresh_at; never outputs/weights/inventory',
                    'resourcePreparation':'finite isolated level20/coin1000000/diamond200/original red selection box1; no food, ingredients or warrior instance injection',
                    'pending':False}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
