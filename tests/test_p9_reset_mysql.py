"""P9 original paid pet reset through isolated MySQL and signed binary HTTP."""
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
SOURCE = Path(os.environ.get('HYJS_P9_SOURCE', ROOT/'build/candidates/p9-reset-20261014'))

@unittest.skipUnless(os.environ.get('HYJS_P9_MYSQL_TEST') == '1', '显式启用P9MySQL回归')
class P9ResetMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_paid_reset_and_restart(self):
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
                '^TestP9NativePetReset$','-count=1','-v'],
                cwd=SOURCE/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(password,'<redacted>'))
            self.assertIn('--- PASS: TestP9NativePetReset',result.stdout)
            cases=[v.split('=== RUN   ',1)[1]for v in result.stdout.splitlines()if v.startswith('=== RUN   ')and '/'in v]
            self.assertEqual(len(cases),1)
            engines=root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p9_%';")
            self.assertEqual(engines.strip(),'InnoDB')
            report={'date':'2026-10-14','candidateGroup':'P9-native-pet-reset','mysqlTest':'passed',
                'goDomainNames':cases,'tablesEngine':'InnoDB','donorRefundAndBagFailureRollback':'passed',
                'missingLegacyInvestmentProtection':'passed','copyIntimacyNotRefunded':'passed',
                'productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p9/reset-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


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
                process=subprocess.Popen([binary,'--http-port','0','--tcp-port','0','--tables',str(ROOT/'analysis/tables')],cwd=SOURCE/'hyjs-server',env=service_env,
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
            try:
                start();uid='p9-reset-native-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原精灵重置链','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=75,noviceGuideSchedule=7,coin=1000000,diamond=200)
                g['items']=[{'id':'item_124','num':10},{'id':'item_132','num':10}];persist(login,p,g)
                first=call('CSPetFirstRecruit',{},login,96000,'SCPetFirstRecruit').get.pets[0]
                initial=profile(login)
                call('CSPetLevelup',{'guid':first.guid,'cost':[{'id':'item_124','num':1}]},login,96001,'SCPetLevelup')
                call('CSPetEat',{'guid':first.guid,'cost':[{'id':'item_132','num':2}]},login,96002,'SCPetEat')
                call('CSFormationOn',{'formatPos':1,'id':first.id,'guid':first.guid},login,96003,'SCFormationOn')
                guarded=profile(login)
                opcode,_=self.stamped('CSPetReset',{'guid':first.guid},login,96004)
                self.assertEqual(opcode,BY_NAME['ErrorMessage']);self.assertEqual(profile(login),guarded)
                call('CSFormationOff',{'formatPos':1,'id':first.id,'guid':first.guid},login,96005,'SCFormationOff')
                req={'guid':first.guid}
                with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:
                    replies=list(pool.map(lambda _:call('CSPetReset',req,login,96006,'SCPetReset'),range(8)))
                for reply in replies:self.assertEqual(reply,replies[0])
                reset=replies[0]
                self.assertEqual([(v.id,v.num)for v in reset.cost],[('diamond',50),(first.id,1)])
                self.assertEqual(reset.cost[1].guid,first.guid)
                self.assertEqual(len(reset.get.pets),1)
                new=reset.get.pets[0];self.assertNotEqual(new.guid,first.guid)
                self.assertEqual((new.id,new.level,new.exp,new.eat.level,new.eat.exp,new.breached.level),(first.id,1,0,0,0,0))
                final=profile(login)
                self.assertEqual(final[0]['coin'],initial[0]['coin']);self.assertEqual(final[0]['diamond'],initial[0]['diamond']-50)
                self.assertEqual({v['id']:v['num']for v in final[1]['items']},{v['id']:v['num']for v in initial[1]['items']})
                self.assertEqual(pet(login,96007),new)
                self.assertEqual(sql("SELECT COUNT(*) FROM p9_pet_retired WHERE guid='"+first.guid+"';").strip(),'1')
                other=self.login('p9-reset-foreign-'+secrets.token_hex(4))
                self.send('CSPlayerCreate',{'playerName':'归属隔离','heroId':'hero_043'},other)
                for who,request in [(login,req),(other,{'guid':new.guid}),(login,{'guid':' '})]:
                    opcode,_=self.stamped('CSPetReset',request,who,96008)
                    self.assertEqual(opcode,BY_NAME['ErrorMessage'])
                self.assertEqual(profile(login),final)
                opcode,_=self.stamped('CSPetLevelup',{'guid':first.guid,'cost':[{'id':'item_124','num':1}]},login,96001)
                self.assertEqual(opcode,BY_NAME['ErrorMessage']);self.assertEqual(profile(login),final)
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSPetReset',req,login,96006,'SCPetReset'),reset)
                self.assertEqual(pet(login,96009),new);self.assertEqual(profile(login),final)
                growth_req={'guid':new.guid,'cost':[{'id':'item_124','num':1}]}
                grown=call('CSPetLevelup',growth_req,login,96010,'SCPetLevelup')
                current=profile(login)
                opcode,_=self.stamped('CSPetReset',req,login,96006)
                self.assertEqual(opcode,BY_NAME['ErrorMessage']);self.assertEqual(profile(login),current)
                self.assertEqual(call('CSPetLevelup',growth_req,login,96010,'SCPetLevelup'),grown)
                return {'networkRoute':'original signed binary HTTP passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'eightConcurrentResetSingleFeeFreshGUID':'passed','guardedAndForeignAndRetiredRejected':'passed',
                    'originalDeleteOldAndAddNewResponse':'passed','serverProcessRestart':'passed',
                    'coldReceiptNoResurrection':'passed','retiredGrowthAndLaterChangedResetProjectionRejected':'passed','resourcePreparation':'isolated level75, coin1M, diamond200, two materials10 each; original first recruit and actual training',
                    'nativeAcceptance':'original phone reset entry remains pending'}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
