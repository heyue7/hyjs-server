"""P9 original pet fragments, focused isolated MySQL and native HTTP verification."""
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
class P9FragmentMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_fragment_source_combine_sell_restart(self):
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
                '^(TestP9PetFragments|TestP9PetIntimacy)$/^(original|actual)','-count=1','-v'],
                cwd=ROOT/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(password,'<redacted>'))
            self.assertIn('--- PASS: TestP9PetFragments',result.stdout)
            self.assertIn('--- PASS: TestP9PetIntimacy',result.stdout)
            cases=[v.split('=== RUN   ',1)[1]for v in result.stdout.splitlines()if v.startswith('=== RUN   ')and '/'in v]
            self.assertEqual(len(cases),4)
            engines=root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p9_%';")
            self.assertEqual(engines.strip(),'InnoDB')
            report={'date':'2026-10-13','candidateGroup':'P9-original-pet-fragments','mysqlTest':'passed',
                'goDomainSubtests':4,'goDomainNames':cases,'tablesEngine':'InnoDB',
                'sixConcurrentOriginalCombineUniqueGUID':'passed','originalFragmentCoinSale':'passed',
                'capacityAndMixedBatchRollback':'passed','sourceBoxItem155ToFragmentsToOwnedPet':'passed',
                'foodNaturalSource':'isolated Saturday business clock only: original dailyCopy31/34 each6 -> real feed180; P0 client settlement compatibility, no current-date native authority claim',
                'ordinaryRecruitAndRandomIntimacy':'explicitly unsupported pending original source rules',
                'productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p9/fragment-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

    def native_network(self,database,env,root_sql):
        with tempfile.TemporaryDirectory(prefix='hyjs-p9-fragments-')as temp:
            binary=str(Path(temp)/'hyjs-p9-fragments')
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
            with (ROOT/'analysis/tables/TableFrag.tsv').open()as handle:rows=list(csv.reader(handle,delimiter='\t'))
            fragments={v[0]:dict(zip(rows[1],v))for v in rows[5:]if v and v[0]}
            try:
                start();uid='p9-fragment-native-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原精灵碎片链','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=34,noviceGuideSchedule=7)
                g['items']=[{'id':'item_155','num':1000}];persist(login,p,g)
                first_box=None
                for batch in range(10):
                    box=call('CSItemUse',{'costItem':{'id':'item_155','num':100}},login,93000+batch,'SCItemUse')
                    self.assertEqual(sum(v.num for v in box.get.simps),500)
                    self.assertTrue(all(v.id.startswith('frag_pet_')for v in box.get.simps))
                    if first_box is None:first_box=box
                listed=call('CSFragmentList',{},login,93010,'SCFragmentList')
                self.assertEqual(sum(v.num for v in listed.fragPetList),5000)
                eligible=sorted((v for v in listed.fragPetList if v.num>=int(fragments[v.id]['num'])),key=lambda v:v.id)
                self.assertTrue(eligible,'bounded original source box draws must supply at least one complete50-fragment pet')
                selected=eligible[0];target=fragments[selected.id]['target'];needed=int(fragments[selected.id]['num'])
                request={'targetId':target}
                combined=call('CSCombine',request,login,93011,'SCCombine')
                self.assertEqual([(v.id,v.num)for v in combined.cost],[(selected.id,needed)])
                self.assertEqual(len(combined.get.pets),1);pet=combined.get.pets[0]
                self.assertEqual((pet.id,pet.level,pet.eat.level,pet.breached.level),(target,1,0,0));self.assertTrue(pet.guid)
                owned=call('CSPetList',{},login,93012,'SCPetList')
                self.assertEqual(list(owned.petList),[pet])
                self.assertEqual(call('CSCombine',request,login,93011,'SCCombine'),combined)
                listed=call('CSFragmentList',{},login,93013,'SCFragmentList')
                stock={v.id:v.num for v in listed.fragPetList}
                self.assertEqual(stock.get(selected.id,0),selected.num-needed)
                sell_id=next(key for key in sorted(stock)if stock[key]>0)
                sale_request={'sells':[{'id':sell_id,'num':1}]}
                sold=call('CSPetFragSell',sale_request,login,93014,'SCPetFragSell')
                price=int(fragments[sell_id]['price'])
                self.assertEqual([(v.id,v.num)for v in sold.get.simps],[('coin',price)])
                self.assertEqual([(v.id,v.num)for v in sold.cost],[(sell_id,1)])
                self.assertEqual(profile(login)[0]['coin'],price)
                self.assertEqual(profile(login)[0].get('coinpet',0),0)
                self.assertEqual(call('CSPetFragSell',sale_request,login,93014,'SCPetFragSell'),sold)
                before=profile(login)
                self.assertEqual(self.stamped('CSPetFragSell',{'sells':[{'id':sell_id,'num':1},{'id':sell_id,'num':1}]},login,93015)[0],10001)
                self.assertEqual(profile(login),before)
                p,g=profile(login);p['petBagSize']=1;persist(login,p,g);before=profile(login)
                full_id=next(v.id for v in eligible if stock.get(v.id,0)-(1 if v.id==sell_id else 0)>=int(fragments[v.id]['num']))
                self.assertEqual(self.stamped('CSCombine',{'targetId':fragments[full_id]['target']},login,93016)[0],10001)
                self.assertEqual(profile(login),before)
                expected=call('CSFragmentList',{},login,93017,'SCFragmentList')
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSPetList',{},login,93018,'SCPetList'),owned)
                self.assertEqual(call('CSFragmentList',{},login,93019,'SCFragmentList'),expected)
                self.assertEqual(call('CSCombine',request,login,93011,'SCCombine'),combined)
                self.assertEqual(call('CSPetFragSell',sale_request,login,93014,'SCPetFragSell'),sold)
                self.assertEqual(call('CSItemUse',{'costItem':{'id':'item_155','num':100}},login,93000,'SCItemUse'),first_box)
                self.assertEqual(profile(login)[0]['coin'],price)
                self.assertEqual(call('CSFragmentList',{},login,93020,'SCFragmentList'),expected)
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'nativeItem155ActualOriginalFragments':'passed','nativeCSCombineUniquePetAndOriginalCost':'passed',
                    'nativeFragmentSaleOriginalCoinAndReceipt':'passed','nativeCapacityAndDuplicateBatchRollback':'passed',
                    'serverProcessRestart':'passed','nativeSourceCombineSaleReceiptAfterRestart':'passed',
                    'resourcePreparation':'finite isolated level34, original item155 boxes1000 and capacity1; no fragment or pet instance injection'}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
