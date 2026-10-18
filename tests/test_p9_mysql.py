"""P9精灵首次获得与原扭蛋实例，专属MySQL用户/库，定点回归并清理。"""
import json
import hashlib
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
class P9MySQLTests(unittest.TestCase):
    send = GoFormationTests.send
    _unauthed_send = GoFormationTests._unauthed_send
    login = GoFormationTests.login
    stamped = GoFormationTests.stamped

    def test_original_pet_instances_transaction_and_restart(self):
        suffix = secrets.token_hex(6)
        database = 'hyjs_test_p9_' + suffix
        user = 'p9_' + suffix
        password = secrets.token_hex(24)

        def root_sql(query):
            self.assertTrue(database.startswith('hyjs_test_p9_'))
            result = subprocess.run(['rtk', 'proxy', 'docker', 'exec', '-i', 'hyjs-mysql', 'sh', '-c',
                                     'MYSQL_PWD="$MYSQL_ROOT_PASSWORD" exec mysql -uroot --batch --raw --skip-column-names'],
                                    input=query, text=True, capture_output=True, timeout=30)
            self.assertEqual(result.returncode, 0, 'P9 pets isolated database administration failed')
            return result.stdout

        started = time.monotonic()
        try:
            root_sql(f"CREATE DATABASE `{database}` CHARACTER SET utf8mb4; CREATE USER '{user}'@'%' IDENTIFIED BY '{password}'; GRANT ALL ON `{database}`.* TO '{user}'@'%';")
            env = dict(os.environ, HYJS_P9_MYSQL_DATABASE=database, HYJS_MYSQL_HOST='127.0.0.1:33068',
                       HYJS_MYSQL_USER=user, HYJS_MYSQL_PASSWORD=password)
            result = subprocess.run(['rtk', 'proxy', 'go', 'test', './service', '-run', '^TestP9(PetInstances|GuardGrowth|PetEquipment|PetRecycle|PetIntimacy)$|^TestP9FirstNativeProjection$', '-count=1', '-v'],
                                    cwd=ROOT / 'hyjs-server', env=env, text=True, capture_output=True, timeout=120)
            summary = '\n'.join(line for line in result.stdout.splitlines()
                                if line.startswith(('=== RUN', '--- PASS', '--- FAIL', 'ok ', 'FAIL')))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('--- PASS: TestP9PetInstances', summary)
            self.assertIn('--- PASS: TestP9GuardGrowth', summary)
            self.assertIn('--- PASS: TestP9PetEquipment', summary)
            self.assertIn('--- PASS: TestP9PetRecycle', summary)
            self.assertIn('--- PASS: TestP9PetIntimacy', summary)
            self.assertIn('--- PASS: TestP9FirstNativeProjection', summary)
            engines = root_sql(f"SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='{database}' AND TABLE_NAME LIKE 'p9_%';")
            self.assertEqual(engines.strip(), 'InnoDB')
            network = self.native_network(database, env, root_sql)
            report = {'date':'2026-10-13','candidateGroup':'P9-native-first-projection','mysqlTest':'passed','goDomainSubtests':17,
                      'firstOriginalPetAnd34Gate':'passed','eightConcurrentFirstReceipt':'passed',
                      'nativeBoxWeightedPetGUIDs':'passed','originalRedPetFourEquipmentSlots':'passed',
                      'capacityBatchRollback':'passed','guardBoundActualBattleProperty':'passed',
                      'sixConcurrentUpgradeExactCostsAndLedger':'passed','originalBreachCostsAndConstants':'passed',
                      'sameIdGuidDonorGuardGrowthProtection':'passed','databaseConnectionReopen':'passed','tablesEngine':'InnoDB',
                      'ordinaryRecruitPool':'explicitly unsupported; original server pool and pity unverified',
                      'productionSaveModified':False,'productionDeployment':False}
            report.update(network)
        finally:
            root_sql(f"DROP DATABASE IF EXISTS `{database}`; DROP USER IF EXISTS '{user}'@'%';")
        remaining = root_sql(f"SELECT COUNT(*) FROM information_schema.schemata WHERE schema_name='{database}'; SELECT COUNT(*) FROM mysql.user WHERE user='{user}'; SELECT COUNT(*) FROM mysql.db WHERE Db='{database}';")
        self.assertEqual(remaining.split(), ['0', '0', '0'])
        report.update(dedicatedDatabaseUserGrantCleanup='passed', testSeconds=round(time.monotonic() - started, 3))
        (ROOT / 'analysis/p9/instance-validation.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')

    def native_network(self, database, env, root_sql):
        with tempfile.TemporaryDirectory(prefix='hyjs-p9-native-') as temp:
            binary = str(Path(temp) / 'hyjs-p9-pets')
            built = subprocess.run(['rtk', 'proxy', 'go', 'build', '-o', binary, '.'], cwd=ROOT / 'hyjs-server', capture_output=True, timeout=120)
            self.assertEqual(built.returncode, 0, 'P9 pets isolated build failed')
            process = None
            service_env = dict(env, HYJS_MYSQL_DATABASE=database)

            def start():
                nonlocal process
                process = subprocess.Popen([binary, '--http-port', '0', '--tcp-port', '0'], cwd=ROOT / 'hyjs-server', env=service_env,
                                           stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
                with selectors.DefaultSelector() as ready:
                    ready.register(process.stdout, selectors.EVENT_READ)
                    self.assertTrue(ready.select(90), 'P9 pets isolated startup timeout')
                    line = process.stdout.readline()
                found = re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)', line)
                self.assertIsNotNone(found, 'P9 pets isolated startup rejected')
                self.service = types.SimpleNamespace(http_port=int(found[1]), tcp_port=int(found[2]))

            def stop():
                nonlocal process
                if process is not None and process.poll() is None:
                    process.terminate()
                    try:
                        process.wait(timeout=10)
                    except subprocess.TimeoutExpired:
                        process.kill(); process.wait(timeout=10)
                if process is not None and process.stdout:
                    process.stdout.close()
                process = None

            def sql(query):
                return root_sql(f'USE `{database}`; ' + query)

            def profile(login):
                raw = sql('SELECT CONCAT(HEX(player),"|",HEX(game)) FROM profiles WHERE id=' + str(login.playerId) + ';').strip().split('|')
                return [json.loads(bytes.fromhex(value)) for value in raw]

            def call(name, fields, login, stamp, expected):
                opcode, result = self.stamped(name, fields, login, stamp)
                self.assertEqual(opcode,BY_NAME[expected],str(result))
                return result
            try:
                start()
                uid='p9-native-'+secrets.token_hex(4)
                login=self.login(uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'原帧精灵回归','heroId':'hero_043'},login)[0],BY_NAME['SCPlayerCreate'])
                p,g=profile(login)
                p.update(level=33,noviceGuideSchedule=7)
                g['items']=[{'id':'item_154','num':2},{'id':'item_153','num':1},{'id':'item_126','num':5000},{'id':'item_124','num':20000},{'id':'item_139','num':1000},{'id':'item_144','num':1000}]
                p['coin']=10000000
                def persist():
                    values=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[p,g]]
                    sql("UPDATE profiles SET player=X'"+values[0]+"',game=X'"+values[1]+"' WHERE id="+str(login.playerId)+';')
                persist()
                self.assertEqual(len(call('CSPetList',{},login,71000,'SCPetList').petList),0)
                saved=profile(login)
                self.assertEqual(self.stamped('CSPetFirstRecruit',{},login,71001)[0],10001)
                self.assertEqual(profile(login),saved)
                self.assertEqual(call('CSPetRecruitGet',{},login,71600,'SCPetRecruitGet').recruit.reward,'')
                p['level']=34;persist()
                hint=call('CSPetRecruitGet',{},login,71601,'SCPetRecruitGet').recruit
                self.assertEqual((hint.reward,hint.num,hint.mustTimes,hint.nextFree),('pet_002,1',0,0,0))
                first=call('CSPetFirstRecruit',{},login,71002,'SCPetFirstRecruit')
                self.assertEqual(len(first.get.pets),1)
                pet=first.get.pets[0]
                self.assertEqual((pet.id,pet.level,pet.initProp),('pet_002',1,'PetTalent_11'))
                self.assertTrue(pet.guid)
                self.assertEqual(call('CSPetList',{},login,71003,'SCPetList').petList[0],pet)
                self.assertEqual(call('CSPetRecruitGet',{},login,71602,'SCPetRecruitGet').recruit.reward,'')
                self.assertEqual(call('CSPetFirstRecruit',{},login,71002,'SCPetFirstRecruit'),first)
                self.assertEqual(self.stamped('CSPetFirstRecruit',{},login,71004)[0],10001)
                box=call('CSItemUse',{'costItem':{'id':'item_154','num':2}},login,71005,'SCItemUse')
                self.assertEqual(len(box.get.pets),2)
                red=call('CSItemUse',{'costItem':{'id':'item_153','num':1}},login,71006,'SCItemUse').get.pets[0]
                self.assertEqual(len(red.petEquipList),4)
                pets=call('CSPetList',{},login,71007,'SCPetList')
                self.assertEqual(len(pets.petList),4)
                self.assertEqual(len({v.guid for v in pets.petList}),4)
                self.assertEqual(self.stamped('CSPetRecruit',{'free':0,'numType':1},login,71008)[0],10001)
                baseline_login=self.login('p9-baseline-'+secrets.token_hex(4))
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'守护属性基线','heroId':'hero_043'},baseline_login)[0],BY_NAME['SCPlayerCreate'])
                base_p,base_g=profile(baseline_login);base_p.update(level=34,noviceGuideSchedule=7)
                base_values=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[base_p,base_g]]
                sql("UPDATE profiles SET player=X'"+base_values[0]+"',game=X'"+base_values[1]+"' WHERE id="+str(baseline_login.playerId)+';')
                call('CSCopyEnter',{'chapterId':'1','missionId':'1'},baseline_login,71200,'SCCopyEnter')
                baseline_fight=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},baseline_login,71201,'SCCopyFightStart')
                baseline_att=baseline_fight.battlePlayer.formation[0].prop.att
                baseline_hp=baseline_fight.battlePlayer.formation[0].prop.hp
                equipment_uid='p9-equipment-'+secrets.token_hex(4)
                equipment_login=self.login(equipment_uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'红装原帧回归','heroId':'hero_043'},equipment_login)[0],BY_NAME['SCPlayerCreate'])
                ep,eg=profile(equipment_login);ep.update(level=34,coin=100000000,noviceGuideSchedule=7)
                eg['items']=[{'id':'item_153','num':1},{'id':'item_124','num':20000}]+[{'id':f'item_{n:03}','num':1000}for n in range(374,399)]
                raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[ep,eg]]
                sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(equipment_login.playerId)+';')
                red_owned=call('CSItemUse',{'costItem':{'id':'item_153','num':1}},equipment_login,71300,'SCItemUse').get.pets[0]
                import csv
                def table(name):
                    with (ROOT/'analysis/tables'/('Table'+name+'.tsv')).open()as handle: rows=list(csv.reader(handle,delimiter='\t'))
                    return [dict(zip(rows[1],v))for v in rows[5:]if v and v[0]]
                gain=sum(int(v['exp'])for v in table('PetExp')if v['quality']=='23'and 1<=int(v['level'])<20)
                grown_red=call('CSPetLevelup',{'guid':red_owned.guid,'cost':[{'id':'item_124','num':(gain+99)//100}]},equipment_login,71301,'SCPetLevelup').pet
                self.assertEqual(grown_red.level,20)
                call('CSFormationOn',{'id':grown_red.id,'guid':grown_red.guid,'formatPos':1},equipment_login,71302,'SCFormationOn')
                eid=grown_red.petEquipList[0].equipId
                equip_fields={'petGuid':grown_red.guid,'equipId':eid,'num':5}
                strong=call('CSPetEquipStrengthOne',equip_fields,equipment_login,71303,'SCPetEquipStrengthOne')
                self.assertEqual(strong.petEquip.strengthLevel,5)
                expected_cost=sum(int(v['cost'].split(',')[1])for v in table('PetEquipStrength')if v['equipId']==eid and 1<=int(v['level'])<=5)
                self.assertEqual(sum(v.num for v in strong.cost if v.id=='coin'),expected_cost)
                self.assertEqual(call('CSPetEquipStrengthOne',equip_fields,equipment_login,71303,'SCPetEquipStrengthOne'),strong)
                strong10=call('CSPetEquipStrengthOne',equip_fields,equipment_login,71304,'SCPetEquipStrengthOne')
                self.assertEqual(strong10.petEquip.strengthLevel,10)
                prior=profile(equipment_login)
                self.assertEqual(self.stamped('CSPetEquipStrengthOne',equip_fields,equipment_login,71305)[0],10001)
                self.assertEqual(profile(equipment_login),prior)
                pair={'petGuid':grown_red.guid,'equipId':eid}
                advanced=call('CSPetEquipAdvance',pair,equipment_login,71306,'SCPetEquipAdvance')
                awoken=call('CSPetEquipAwake',pair,equipment_login,71307,'SCPetEquipAwake')
                self.assertEqual((advanced.petEquip.advanceLevel,awoken.petEquip.awakeLevel),(1,1))
                def cost_set(text):return sorted((v.split(',')[0],int(v.split(',')[1]))for v in text.split(';')if v)
                ar=next(v for v in table('PetEquipAdvance')if v['equipId']==eid and v['level']=='1')
                wr=next(v for v in table('PetEquipAwake')if v['equipId']==eid and v['level']=='1')
                self.assertEqual(sorted((v.id,v.num)for v in advanced.cost),cost_set(ar['cost']))
                self.assertEqual(sorted((v.id,v.num)for v in awoken.cost),cost_set(wr['cost']))
                equip_query=call('CSGetPetEquipInfo',{'petGuid':[grown_red.guid]},equipment_login,71308,'SCGetPetEquipInfo')
                self.assertEqual(len(equip_query.petEquip),4)
                self.assertEqual(self.stamped('CSGetPetEquipInfo',{'petGuid':[grown_red.guid]},login,71309)[0],10001)
                call('CSCopyEnter',{'chapterId':'1','missionId':'1'},equipment_login,71310,'SCCopyEnter')
                ef=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},equipment_login,71311,'SCCopyFightStart')
                def hp(text):return sum(float(v.split(',')[1])for v in text.split(';')if v and v.split(',')[0]=='hp')
                talent=next(v for v in table('PetTalent')if v['petTalentId']==grown_red.initProp and v['talentLevel']=='0')
                expected_hp=hp(talent['talent'])+hp(talent['talentGrow'])*19
                expected_hp+=sum(hp(next(v for v in table('PetEquip')if v['id']==eq.equipId)['prop'])for eq in grown_red.petEquipList)
                sr=next(v for v in table('PetEquipStrength')if v['equipId']==eid and v['level']=='10')
                expected_hp+=hp(sr['prop'])+hp(ar['prop'])+hp(wr['prop'])
                self.assertAlmostEqual(ef.battlePlayer.formation[0].prop.hp-baseline_hp,expected_hp,places=7)
                recycle_uid='p9-recycle-'+secrets.token_hex(4)
                recycle_login=self.login(recycle_uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'精灵原价出售','heroId':'hero_043'},recycle_login)[0],BY_NAME['SCPlayerCreate'])
                rp,rg=profile(recycle_login);rp.update(level=34,coin=100,noviceGuideSchedule=7)
                rg['items']=[{'id':'item_153','num':1},{'id':'item_154','num':2}]
                raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[rp,rg]]
                sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(recycle_login.playerId)+';')
                sell_red=call('CSItemUse',{'costItem':{'id':'item_153','num':1}},recycle_login,71400,'SCItemUse').get.pets[0]
                sell_orange=call('CSItemUse',{'costItem':{'id':'item_154','num':2}},recycle_login,71401,'SCItemUse').get.pets
                call('CSFormationOn',{'id':sell_orange[0].id,'guid':sell_orange[0].guid,'formatPos':1},recycle_login,71402,'SCFormationOn')
                prior=profile(recycle_login)
                self.assertEqual(self.stamped('CSPetSell',{'guids':[sell_red.guid,sell_orange[0].guid]},recycle_login,71403)[0],10001)
                self.assertEqual(profile(recycle_login),prior)
                sell_guids=[sell_red.guid,sell_orange[1].guid]
                sold=call('CSPetSell',{'guids':sell_guids},recycle_login,71404,'SCPetSell')
                self.assertEqual({v.guid for v in sold.cost},set(sell_guids))
                self.assertEqual([(v.id,v.num)for v in sold.get.simps],[('coin',1200)])
                self.assertEqual(profile(recycle_login)[0]['coin'],1300)
                self.assertEqual(call('CSPetSell',{'guids':sell_guids},recycle_login,71404,'SCPetSell'),sold)
                surviving=call('CSPetList',{},recycle_login,71405,'SCPetList')
                self.assertEqual([v.guid for v in surviving.petList],[sell_orange[0].guid])
                self.assertEqual(sql('SELECT COUNT(*) FROM p9_pet_retired WHERE player_id='+str(recycle_login.playerId)+';').strip(),'2')
                self.assertEqual(self.stamped('CSPetDecompose',{'guids':[sell_orange[0].guid]},recycle_login,71406)[0],10001)
                self.assertEqual(self.stamped('CSPetReset',{'guid':sell_orange[0].guid},recycle_login,71407)[0],10001)
                food_uid='p9-food-'+secrets.token_hex(4)
                food_login=self.login(food_uid)
                self.assertEqual(self.send('CSPlayerCreate',{'playerName':'精灵原食物回归','heroId':'hero_043'},food_login)[0],BY_NAME['SCPlayerCreate'])
                fp,fg=profile(food_login);fp.update(level=34,noviceGuideSchedule=7)
                fg['items']=[{'id':'item_129','num':32},{'id':'item_128','num':2},{'id':'item_133','num':1},{'id':'item_138','num':1}]
                raw=[json.dumps(v,ensure_ascii=False,separators=(',',':')).encode().hex()for v in[fp,fg]]
                sql("UPDATE profiles SET player=X'"+raw[0]+"',game=X'"+raw[1]+"' WHERE id="+str(food_login.playerId)+';')
                food_pet=call('CSPetFirstRecruit',{},food_login,71500,'SCPetFirstRecruit').get.pets[0]
                eaten_fields={'guid':food_pet.guid,'cost':[{'id':'item_129','num':1}]}
                eaten=call('CSPetEat',eaten_fields,food_login,71501,'SCPetEat')
                self.assertEqual((eaten.pet.eat.level,eaten.pet.eat.exp),(0,20))
                self.assertEqual(call('CSPetEat',eaten_fields,food_login,71501,'SCPetEat'),eaten)
                different=call('CSPetEat',{'guid':food_pet.guid,'cost':[{'id':'item_128','num':1}]},food_login,71502,'SCPetEat')
                self.assertEqual(different.pet.eat.exp,30)
                prior=profile(food_login)
                self.assertEqual(self.stamped('CSPetEat',{'guid':food_pet.guid,'cost':[{'id':'item_129','num':30}]},food_login,71503)[0],10001)
                self.assertEqual(profile(food_login),prior)
                fed=call('CSPetEat',{'guid':food_pet.guid,'cost':[{'id':'item_129','num':29}]},food_login,71504,'SCPetEat')
                self.assertEqual((fed.pet.eat.level,fed.pet.eat.exp),(0,610))
                shown=call('CSPetHudongShow',{'guid':food_pet.guid},food_login,71505,'SCPetHudongShow')
                self.assertEqual(shown.pet,fed.pet)
                self.assertEqual(self.stamped('CSPetHudongShow',{'guid':food_pet.guid},login,71506)[0],10001)
                self.assertEqual(self.stamped('CSPetHudong',{'guid':food_pet.guid,'type':1},food_login,71507)[0],10001)
                guarded=call('CSFormationOn',{'id':pet.id,'guid':pet.guid,'formatPos':1},login,71100,'SCFormationOn')
                self.assertEqual(guarded.formationItem.things[-1].pos,7)
                grown=call('CSPetLevelup',{'guid':pet.guid,'cost':[{'id':'item_126','num':2}]},login,71101,'SCPetLevelup')
                self.assertEqual((grown.pet.level,grown.pet.exp),(2,500))
                self.assertEqual(grown.cost[-1].num,2000)
                self.assertEqual(profile(login)[0]['coin'],9998000)
                self.assertEqual(call('CSPetLevelup',{'guid':pet.guid,'cost':[{'id':'item_126','num':2}]},login,71101,'SCPetLevelup'),grown)
                # Source test accounts contain only bounded prerequisites; actual
                # native item costs grow the same obtained GUID to original level30.
                levels=[]
                import csv
                with (ROOT/'analysis/tables/TablePetExp.tsv').open() as table:
                    rows=list(csv.reader(table,delimiter='	'))
                for values in rows[5:]:
                    if values and values[0]:
                        row=dict(zip(rows[1],values))
                        if row['quality']=='20' and 2<=int(row['level'])<30:levels.append(int(row['exp']))
                gain=sum(levels)-grown.pet.exp
                costs=[]
                if gain//1000:costs.append({'id':'item_126','num':gain//1000})
                if gain%1000:costs.append({'id':'item_124','num':(gain%1000+99)//100})
                level30=call('CSPetLevelup',{'guid':pet.guid,'cost':costs},login,71102,'SCPetLevelup')
                self.assertEqual(level30.pet.level,30)
                breach=call('CSPetBreached',{'guid':pet.guid},login,71103,'SCPetBreached')
                self.assertEqual(breach.pet.breached.level,1)
                self.assertEqual([(c.id,c.num)for c in breach.cost],[('item_139',50),('item_144',15)])
                pets=call('CSPetList',{},login,71104,'SCPetList')
                # Actual original story entry emits the compiled BattlePlayer.
                call('CSCopyEnter',{'chapterId':'1','missionId':'1'},login,71107,'SCCopyEnter')
                fight=call('CSCopyFightStart',{'chapterId':'1','missionId':'1','taskId':'1'},login,71105,'SCCopyFightStart')
                self.assertTrue(fight.battlePlayer.formation)
                self.assertAlmostEqual(fight.battlePlayer.formation[0].prop.att-baseline_att,673,places=7)
                saved=profile(login)
                self.assertEqual(self.stamped('CSFormationOff',{'id':pet.id,'guid':pet.guid,'formatPos':1},login,71106)[0],10001)
                self.assertEqual(profile(login),saved)
                stop();start();login=self.login(uid)
                self.assertEqual(call('CSPetRecruitGet',{},login,71603,'SCPetRecruitGet').recruit.reward,'')
                food_login=self.login(food_uid)
                self.assertEqual(call('CSPetHudongShow',{'guid':food_pet.guid},food_login,71508,'SCPetHudongShow'),shown)
                self.assertEqual(call('CSPetEat',eaten_fields,food_login,71501,'SCPetEat'),eaten)
                self.assertEqual(call('CSPetList',{},food_login,71509,'SCPetList').petList[0],fed.pet)
                recycle_login=self.login(recycle_uid)
                self.assertEqual(call('CSPetList',{},recycle_login,71408,'SCPetList'),surviving)
                self.assertEqual(call('CSPetSell',{'guids':sell_guids},recycle_login,71404,'SCPetSell'),sold)
                self.assertEqual(profile(recycle_login)[0]['coin'],1300)
                equipment_login=self.login(equipment_uid)
                self.assertEqual(call('CSGetPetEquipInfo',{'petGuid':[grown_red.guid]},equipment_login,71312,'SCGetPetEquipInfo'),equip_query)
                self.assertEqual(call('CSPetEquipStrengthOne',equip_fields,equipment_login,71303,'SCPetEquipStrengthOne'),strong)
                self.assertEqual(call('CSPetEquipAwake',pair,equipment_login,71307,'SCPetEquipAwake'),awoken)
                self.assertEqual(call('CSPetList',{},login,71009,'SCPetList'),pets)
                self.assertEqual(call('CSPetFirstRecruit',{},login,71002,'SCPetFirstRecruit'),first)
                self.assertEqual(call('CSItemUse',{'costItem':{'id':'item_154','num':2}},login,71005,'SCItemUse'),box)
                self.assertEqual(call('CSPetList',{},login,71010,'SCPetList'),pets)
                return {'networkRoute':'passed','networkBinarySha256':hashlib.sha256(Path(binary).read_bytes()).hexdigest(),
                        'serverProcessRestart':'passed','nativeOriginal34Gate':'passed','nativePetNonemptyList':'passed',
                        'nativeBoxIndependentGUIDs':'passed','nativeFirstAndBoxReceiptAfterRestart':'passed','nativeGuardUpgradeBreachAndLedger':'passed',
                        'nativeBattleProjectionAndCombatMutationGate':'passed',
                        'nativeEquipmentStrengthAdvanceAwakeExactCosts':'passed',
                        'nativeEquipmentActualBattleProperty':'passed','nativeEquipmentOwnedQueryAndRestartReceipt':'passed',
                        'nativeBaseSellOriginalPriceGuidAndGuardProtection':'passed',
                        'nativeSellRetiredSnapshotAndRestartReceipt':'passed',
                        'nativeFoodNatureRatioCostsAndPreunlockBoundary':'passed',
                        'nativeFoodOwnedQueryAndRestartReceipt':'passed',
                        'nativeFirstFactProjectionOriginalFullSpecAndRestart':'passed',
                        'firstNativeAPKButton':'root-owned pending verification; projection alone does not close native UI reachability',
                        'foodRandomUnlockAndInteraction':'explicitly unsupported until original random/critical allocation is verified',
                        'resetAndDecomposeUnverifiedRules':'explicitly unsupported; preserve source instances and investments',
                        'resourcePreparation':'finite isolated level and original pet boxes; no pet instance injection'}
            finally:
                stop()


del GoFormationTests


if __name__ == '__main__':
    unittest.main()
