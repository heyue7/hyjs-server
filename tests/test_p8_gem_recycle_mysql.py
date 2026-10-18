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
class P8GemRecycleMySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_mounted_gem_reset_decompose_restart(self):
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
                '^TestP8GemRecycle$','-count=1','-v'],
                cwd=ROOT/'hyjs-server',env=env,text=True,capture_output=True,timeout=120)
            self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(password,'<redacted>'))
            self.assertIn('--- PASS: TestP8GemRecycle',result.stdout)
            cases=[v.split('=== RUN   ',1)[1]for v in result.stdout.splitlines()if v.startswith('=== RUN   ')and '/'in v]
            self.assertEqual(len(cases),3)
            engines=root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p5_%';")
            self.assertEqual(engines.strip(),'InnoDB')
            report={'date':'2026-10-13','candidateGroup':'P8-P5-auto-unmount-gem-recycling','mysqlTest':'passed',
                'goDomainSubtests':3,'goDomainNames':cases,'tablesEngine':'InnoDB',
                'trackedSwapCurrentGemResetOnly':'passed','basicSixConcurrentDecomposeReturnsGemOnce':'passed',
                'invalidSecondColourAndOverflowBatchRollback':'passed','productionSaveModified':False,'productionDeployment':False}
            report.update(self.native_network(database,env,root_sql))
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining=root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(),['0','0','0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed',testSeconds=round(time.monotonic()-began,3))
        (ROOT/'analysis/p8/gem-recycle-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


    def native_network(self,database,env,root_sql):
        import concurrent.futures
        with tempfile.TemporaryDirectory(prefix='hyjs-p8-gem-recycle-')as temp:
            binary=str(Path(temp)/'hyjs-p8-gem-recycle')
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

            import concurrent.futures
            try:
                start();uid='p8-gem-recycle-native-'+secrets.token_hex(4);login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原自动卸石修复','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login);p.update(level=20,noviceGuideSchedule=7,coin=1000000,diamond=100)
                g['items']=[{'id':'item_119','num':2}]
                g['gems']=[{'id':'MineItem_019','num':1},{'id':'MineItem_020','num':1}]
                persist(login,p,g)
                box_request={'costItem':{'id':'item_119','num':2},'getItemIndex':1}
                box=call('CSItemUse',box_request,login,97500,'SCItemUse')
                self.assertEqual([v.equipId for v in box.get.equips],['equip_037']*2)
                self.assertEqual(len({v.guid for v in box.get.equips}),2)
                first,second=box.get.equips
                def count(key):return sum(v['num']for v in (profile(login)[1].get('gems')or[])if v['id']==key)
                mounted=call('CSGemMount',{'guid':first.guid,'gemId':'MineItem_019','pos':0},login,97501,'SCGemMount')
                self.assertEqual(mounted.equip.gems[0],'MineItem_019')
                strengthened=call('CSEquipStrengthenOneKey',{'guid':first.guid},login,97502,'SCEquipStrengthenOneKey')
                spent=sum(v.num for v in strengthened.cost if v.id=='coin')
                self.assertGreater(spent,0)
                self.assertGreater(strengthened.equip.level,1)
                before_reset=profile(login)[0]['coin']
                swapped=call('CSGemMountOrChange',{'guid':first.guid,'gemId':'MineItem_020'},login,97503,'SCGemMountOrChange')
                self.assertEqual([(v.id,v.num)for v in swapped.res.simps],[('MineItem_019',1)])
                self.assertEqual((count('MineItem_019'),count('MineItem_020')),(1,0))
                reset_request={'guid':first.guid}
                reset=call('CSEquipReBuild',reset_request,login,97504,'SCEquipReBuild')
                gains={v.id:v.num for v in reset.get.simps}
                self.assertEqual(gains,{'coin':spent,'MineItem_020':1})
                self.assertEqual(len(reset.get.equips),1)
                self.assertFalse(reset.get.equips[0].gems)
                self.assertEqual(reset.get.equips[0].level,1)
                self.assertEqual(profile(login)[0]['coin'],before_reset+spent)
                self.assertEqual(profile(login)[0]['diamond'],50)
                self.assertEqual((count('MineItem_019'),count('MineItem_020')),(1,1))
                self.assertEqual(call('CSEquipReBuild',reset_request,login,97504,'SCEquipReBuild'),reset)
                call('CSGemMount',{'guid':second.guid,'gemId':'MineItem_019','pos':0},login,97505,'SCGemMount')
                decompose_request={'guid':[second.guid]}
                with concurrent.futures.ThreadPoolExecutor(max_workers=6)as pool:
                    replies=list(pool.map(lambda _:call('CSEquipDecompose',decompose_request,login,97506,'SCEquipDecompose'),range(6)))
                for value in replies:self.assertEqual(value,replies[0])
                decomposed=replies[0]
                self.assertEqual([(v.id,v.num)for v in decomposed.get.simps if v.id.startswith('MineItem_')],[('MineItem_019',1)])
                self.assertEqual([(v.id,v.guid,v.num)for v in decomposed.cost],[('equip_037',second.guid,1)])
                self.assertEqual((count('MineItem_019'),count('MineItem_020')),(1,1))
                saved=profile(login)
                self.assertEqual([v['guid']for v in saved[1]['equipment']],[first.guid])
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSEquipDecompose',decompose_request,login,97506,'SCEquipDecompose'),decomposed)
                self.assertEqual(call('CSEquipReBuild',reset_request,login,97504,'SCEquipReBuild'),reset)
                self.assertEqual(call('CSItemUse',box_request,login,97500,'SCItemUse'),box)
                self.assertEqual(profile(login),saved)
                self.assertEqual((count('MineItem_019'),count('MineItem_020')),(1,1))
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                    'nativeOriginalRedSelectionToTwoSameIDDifferentGUIDEquips':'passed',
                    'nativeMountStrengthenSwapTrackedResetCurrentGemOnly':'passed','nativeOldBaselineGemNotDuplicated':'passed',
                    'nativeBasicGemDecomposeSixConcurrentReturnsOnce':'passed','nativeOtherSameIDEquipmentPreserved':'passed',
                    'serverProcessRestart':'passed','nativeBoxGemResetDecomposeHistoricalReceipts':'passed',
                    'originalRule':'TableText.recycle_desc_text equipment reset/decompose automatically unmount all gems',
                    'resourcePreparation':'finite isolated level20/coin1000000/diamond100/original red selection119 x2 plus original gems019/020 x1; equipment GUIDs originate actual box, no equipment instance injection',
                    'materialSourceBoundary':'gem inventory is finite isolated preparation for recycle verification; no new mining or gem-box source claimed',
                    'productionSourceStatus':'formal fixed525/fbf8f8f9 does not contain this group; pending root complete-source build/release/native acceptance',
                    'pending':False}
            finally:stop()

del GoFormationTests

if __name__=='__main__':unittest.main()
