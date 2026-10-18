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
from server.codec import BY_NAME
from test_go_formation import GoFormationTests

ROOT = Path(__file__).resolve().parents[1]
SOURCE = Path(os.environ.get('HYJS_P9_SOURCE',ROOT))

@unittest.skipUnless(os.environ.get('HYJS_P9_MYSQL_TEST') == '1', '显式启用P9MySQL回归')
class P9CopyIntimacyMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_copy_sources_sweep_ten_and_restart(self):
        suffix = secrets.token_hex(6)
        database = 'hyjs_test_p9_' + suffix
        user = 'p9_' + suffix
        password = secrets.token_hex(24)
        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p9_'))
            result = subprocess.run(['rtk','proxy','docker','exec','-i','hyjs-mysql','sh','-c',
                'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                input=query,text=True,capture_output=True,timeout=30)
            self.assertEqual(result.returncode,0,'P9 isolated DB administration failed')
            return result.stdout
        began=time.monotonic()
        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env=dict(os.environ,HYJS_P9_MYSQL_DATABASE=database,HYJS_MYSQL_HOST='127.0.0.1:33068',HYJS_MYSQL_USER=user,HYJS_MYSQL_PASSWORD=password)
            result=subprocess.run(['rtk','proxy','go','test','./service','-run',
                '^(TestP9CopyIntimacy|TestP9NativePowerRounding)$','-count=1','-v'],
                cwd=SOURCE/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(password,'<redacted>'))
            self.assertIn('--- PASS: TestP9CopyIntimacy',result.stdout)
            self.assertIn('--- PASS: TestP9NativePowerRounding',result.stdout)
            cases=[v.split('=== RUN   ',1)[1]for v in result.stdout.splitlines()if v.startswith('=== RUN   ')and '/'in v]
            self.assertEqual(len(cases),7)
            engines=root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p9_%';")
            self.assertEqual(engines.strip(),'InnoDB')
            report={'date':'2026-10-13','candidateGroup':'P9-committed-copy-intimacy','mysqlTest':'passed',
                'goDomainSubtests':7,'goDomainNames':cases,'tablesEngine':'InnoDB',
                'sixConcurrentSettlementSingleReward':'passed','tenDistinctSingleSweepCount':'passed',
                'outerSettlementRollback':'passed','randomUnlockDeferredFacts':'passed',
                'emptyGuardSourceRemainsEmpty':'passed','nativeBankersFightPowerRegression':'passed',
                'productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p9/copy-intimacy-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


    def native_network(self,database,env,root_sql):
        import concurrent.futures
        with tempfile.TemporaryDirectory(prefix='hyjs-p9-copy-intimacy-')as temp:
            binary=str(Path(temp)/'hyjs-p9-copy-intimacy')
            build=subprocess.run(['rtk','proxy','go','build','-o',binary,'.'],cwd=SOURCE/'hyjs-server',capture_output=True,timeout=120)
            self.assertEqual(build.returncode,0,'P9 isolated build failed')
            process=None
            service_env=dict(env,HYJS_MYSQL_DATABASE=database)
            def start():
                nonlocal process
                process=subprocess.Popen([binary,'--http-port','0','--tcp-port','0'],cwd=SOURCE/'hyjs-server',env=service_env,
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
            def pet(login,stamp):
                result=call('CSPetList',{},login,stamp,'SCPetList')
                self.assertEqual(len(result.petList),1);return result.petList[0]
            def grant_facts(login):
                text=sql('SELECT completed_count,HEX(facts) FROM p9_pet_copy_grants WHERE player_id='+str(login.playerId)+' ORDER BY source_key;')
                result=[]
                for line in text.splitlines():
                    count,data=line.split('\t');self.assertEqual(int(count),1)
                    result.extend(json.loads(bytes.fromhex(data)))
                return result
            try:
                start();uid='p9-copy-native-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原攻略亲密链','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=75,noviceGuideSchedule=7,stamina=1000,coin=100000,exp=0)
                g['items']=[{'id':'item_129','num':31}];persist(login,p,g)
                first=call('CSPetFirstRecruit',{},login,94000,'SCPetFirstRecruit').get.pets[0]
                guarded=call('CSFormationOn',{'formatPos':1,'id':first.id,'guid':first.guid},login,94001,'SCFormationOn')
                self.assertEqual(pet(login,94002).eat.exp,0)
                call('CSCopyEnter',{'chapterId':'1','missionId':'1'},login,94003,'SCCopyEnter')
                finish=None
                for i,task in enumerate(('1','4')):
                    started=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':task},login,94004+i*2,'SCCopyFightStart')
                    request={'fightId':started.fightId,'result':1,'battlePlayer':started.battlePlayer}
                    finish=call('CSCopyFightEnd',request,login,94005+i*2,'SCCopyFightEnd')
                    self.assertEqual(finish.isCopyComplete,int(task=='4'))
                    self.assertEqual(len(finish.petAdds),int(task=='4'))
                    if task=='4':
                        self.assertEqual((finish.petAdds[0].guid,finish.petAdds[0].addExp,finish.petAdds[0].curExp),(first.guid,2,2))
                        self.assertEqual(call('CSCopyFightEnd',request,login,94007,'SCCopyFightEnd'),finish)
                self.assertEqual(pet(login,94008).eat.exp,2)
                before=profile(login)[0]
                sweep_request={'chapterId':'1','missionId':'1'}
                last_sweep=None
                for i in range(10):
                    stamp=94100+i
                    if i==0:
                        with concurrent.futures.ThreadPoolExecutor(max_workers=6)as pool:
                            replies=list(pool.map(lambda _:call('CSCopySweep',sweep_request,login,stamp,'SCCopySweep'),range(6)))
                        for v in replies:self.assertEqual(v,replies[0])
                        last_sweep=replies[0]
                    else:last_sweep=call('CSCopySweep',sweep_request,login,stamp,'SCCopySweep')
                    self.assertEqual([(v.id,v.num)for v in last_sweep.cost],[('stamina',10)])
                    self.assertEqual([(v.guid,v.addExp,v.curExp)for v in last_sweep.petAdds],[(first.guid,2,4+2*i)])
                after=profile(login)[0]
                self.assertEqual(after['stamina'],before['stamina']-100)
                self.assertEqual(after['coin'],before['coin']+89000)
                self.assertEqual(after['exp'],before['exp']+15000)
                self.assertEqual(pet(login,94120).eat.exp,22)
                self.assertEqual(call('CSCopySweep',sweep_request,login,94109,'SCCopySweep'),last_sweep)
                self.assertEqual(profile(login)[0],after)
                self.assertEqual(sum(f['earned']for f in grant_facts(login)),22)
                food_request={'guid':first.guid,'cost':[{'id':'item_129','num':30}]}
                food=call('CSPetEat',food_request,login,94200,'SCPetEat')
                self.assertEqual(pet(login,94201).eat.exp,622)
                near=call('CSCopySweep',sweep_request,login,94210,'SCCopySweep')
                edge=call('CSCopySweep',sweep_request,login,94211,'SCCopySweep')
                deferred=call('CSCopySweep',sweep_request,login,94212,'SCCopySweep')
                self.assertEqual([(v.addExp,v.curExp)for v in near.petAdds],[(2,624)])
                self.assertEqual([(v.addExp,v.curExp)for v in edge.petAdds],[(1,625)])
                self.assertFalse(deferred.petAdds)
                owned=pet(login,94213)
                self.assertEqual((owned.eat.level,owned.eat.exp,len(owned.randPropList)),(0,625,0))
                facts=grant_facts(login)
                self.assertEqual(sum(f['earned']for f in facts),28)
                self.assertEqual(sum(f['applied']for f in facts),25)
                self.assertEqual(sum(f['deferred']for f in facts),3)
                self.assertEqual(sum(f['capped']for f in facts),0)
                self.assertEqual(sql('SELECT COUNT(*) FROM p9_pet_copy_grants WHERE player_id='+str(login.playerId)+';').strip(),'14')
                final=profile(login)
                stop();start();login=self.login(uid)
                self.assertEqual(pet(login,94300),owned)
                self.assertEqual(call('CSCopyFightEnd',request,login,94007,'SCCopyFightEnd'),finish)
                self.assertEqual(call('CSCopySweep',sweep_request,login,94212,'SCCopySweep'),deferred)
                # Earlier food projected622; the later committed copy now owns625.
                # Reject the expired projection and keep both actual history and current state.
                opcode,_=self.stamped('CSPetEat',food_request,login,94200)
                self.assertEqual(opcode,BY_NAME['ErrorMessage'])
                self.assertEqual(profile(login),final)
                self.assertEqual(grant_facts(login),facts)
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'nativeSuccessfulWholeCopyEarnsTwo':'passed','nativeIntermediateTaskAwardsNone':'passed',
                    'nativeTenSequentialOriginalSweepsEarnTwenty':'passed','nativeConcurrentSweepReceiptOnlyOnce':'passed',
                    'nativeSweepActualCostsAndCoinExp':'passed','nativeBoundaryEarned28Applied25Deferred3':'passed',
                    'serverProcessRestart':'passed','nativeWholeCopySweepFoodReceiptAfterRestart':'passed',
                    'battleBoundary':'P0 client result settlement compatibility; NativeCompared remains closed; no authoritative battle claim',
                    'sourceRule':'completed successful whole story mission and each original paid single sweep; loss/intermediate award unknown',
                    'resourcePreparation':'finite isolated player75, stamina1000, coin100000, item129 x31; original first recruit/guard, no pet or intimacy injection',
                    'nativeAcceptance':'root v15 native first/guard and story 1-2 passed; new copy intimacy remains not deployed/not native-accepted'}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
