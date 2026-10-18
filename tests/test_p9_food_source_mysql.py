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

@unittest.skipUnless(os.environ.get('HYJS_P9_MYSQL_TEST') == '1', '显式启用P9MySQL回归')
class P9FoodSourceMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_current_original_food_source_to_feed_restart(self):
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
            report={'date':'2026-10-13','candidateGroup':'P9-current-date-natural-food-source','mysqlTest':'passed',
                'productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p9/food-current-source-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


    def native_network(self,database,env,root_sql):
        import concurrent.futures
        with tempfile.TemporaryDirectory(prefix='hyjs-p9-food-source-')as temp:
            binary=str(Path(temp)/'hyjs-p9-food-source')
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

            from datetime import datetime,timedelta,timezone
            with (ROOT/'analysis/tables/TableDailyCopyChapter.tsv').open()as handle:rows=list(csv.reader(handle,delimiter='\t'))
            chapters={v[0]:dict(zip(rows[1],v))for v in rows[5:]if v and v[0]}
            with (ROOT/'analysis/tables/TableItem.tsv').open()as handle:rows=list(csv.reader(handle,delimiter='\t'))
            items={v[0]:dict(zip(rows[1],v))for v in rows[5:]if v and v[0]}
            try:
                start();uid='p9-food-current-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原今日食品来源','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=75,noviceGuideSchedule=7,stamina=100,coin=0,exp=0)
                persist(login,p,g)
                first=call('CSPetFirstRecruit',{},login,95000,'SCPetFirstRecruit').get.pets[0]
                call('CSFormationOn',{'formatPos':1,'id':first.id,'guid':first.guid},login,95001,'SCFormationOn')
                info=call('CSDailyCopyState',{},login,95002,'SCDailyCopyState')
                opened=[v for v in info.items if v.type.startswith('PetFood_')and v.isOpen==1]
                self.assertTrue(opened,'current original calendar has no accessible PetFood category')
                chosen=sorted(opened,key=lambda v:v.type)[0]
                chapter=chosen.dailyCopyState[0].ChapterId
                key,amount=chapters[chapter]['Drop'].split(',');amount=int(amount)
                self.assertEqual(amount,6)
                current=datetime.now(timezone(timedelta(hours=8)))
                if current.isoweekday()==2:
                    self.assertEqual((chosen.type,chapter,key),('PetFood_dark','28','item_132'))
                self.assertFalse(any(v['id']==key for v in (profile(login)[1]['items'] or [])))
                started=call('CSDailyCopyFightStart',{'ChapterId':chapter},login,95003,'SCDailyCopyFightStart')
                self.assertEqual(profile(login)[0]['stamina'],70)
                fields={'ChapterId':chapter,'fightId':started.fightId,'isWin':1,'optRecord':{'finalRoundNum':1,'attackerTotalLeftHp':1,'defenderTotalLeftHp':0}}
                result=call('CSDailyCopyFightEnd',fields,login,95004,'SCDailyCopyFightEnd')
                self.assertEqual([(v.id,v.num)for v in result.get.simps],[(key,amount)])
                self.assertEqual((result.isWin,result.lastTime),(1,0))
                self.assertEqual(next(v['num']for v in (profile(login)[1]['items'] or [])if v['id']==key),6)
                self.assertEqual(call('CSDailyCopyFightEnd',fields,login,95004,'SCDailyCopyFightEnd'),result)
                food_request={'guid':first.guid,'cost':[{'id':key,'num':amount}]}
                feed=call('CSPetEat',food_request,login,95005,'SCPetEat')
                self.assertEqual([(v.id,v.num)for v in feed.cost],[(key,amount)])
                # Source TableItem food exp/nature are authoritative inputs; the first
                # pet002 original nature1 doubles only same-nature food.
                expected=int(items[key]['exp']) * amount * (2 if items[key]['nature']=='1'else 1)
                self.assertEqual(feed.pet.eat.exp,expected)
                self.assertEqual(pet(login,95006).eat.exp,expected)
                self.assertFalse(any(v['id']==key and v['num']>0 for v in (profile(login)[1]['items'] or [])))
                self.assertEqual(profile(login)[0]['coin'],0)
                before=profile(login)
                self.assertEqual(self.stamped('CSDailyCopyFightStart',{'ChapterId':chapter},login,95007)[0],10001)
                self.assertEqual(profile(login),before)
                engines=sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+database+"' AND TABLE_NAME LIKE 'p9_%';")
                self.assertEqual(engines.strip(),'InnoDB')
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSDailyCopyFightEnd',fields,login,95004,'SCDailyCopyFightEnd'),result)
                self.assertEqual(call('CSPetEat',food_request,login,95005,'SCPetEat'),feed)
                self.assertEqual(pet(login,95008),feed.pet)
                self.assertEqual(profile(login),before)
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'originalBusinessDateUTC8':current.isoformat(),'originalWeekday':current.isoweekday(),
                    'originalNaturalCategory':chosen.type,'originalChapter':chapter,'originalFoodSource':key,
                    'actualFoodDrop':amount,'actualFeedIntimacy':expected,'actualStaminaCost':30,
                    'nativeActualDailyDropToOwnedFoodToFeed':'passed','sourceQuotaAndHistoricalReceipt':'passed',
                    'serverProcessRestart':'passed','noBusinessClockOverride':True,'noFoodOrPetInjection':True,
                    'tablesEngine':'InnoDB','sourceRule':'actual current original calendar and original TableDailyCopyChapter/TableItem',
                    'battleBoundary':'P0 client result settlement compatibility; NativeCompared remains closed; no authoritative battle claim',
                    'resourcePreparation':'finite isolated player75 and stamina100; original first/guard and real daily food drop; no item injection',
                    'nativeAcceptance':'service original HTTP only; root owns current phone food source/feed acceptance'}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
