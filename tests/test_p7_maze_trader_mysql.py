"""Original54513/54529 merchant on a real accepted Trial defeat; Root SQL lease required.

Finite Player22/initial diamond100 are explicit isolated qualification inputs.
The full existing prepublished local127 enemy is reused without panel mutation.
Private-server floor/catalogue/probability is not asserted. No phone claim.
"""
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import selectors
import shutil
import subprocess
import tempfile
import time
import types
import unittest
from hyjs_managed_mysql import ManagedMySQL
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport
from server.codec import message,BY_NAME

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P7_MAZE_TRADER_MYSQL_TEST')=='1','Root single managed SQL window')
class P7MazeTraderMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def start(self):
        self.process=subprocess.Popen([str(self.binary),'--http-port','0','--tcp-port','0','--tables',str(self.source/'analysis/tables'),'--log-file',str(self.native_log)],cwd=self.source/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector()as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ)
            self.assertTrue(ready.select(90),'native startup timeout')
            line=self.process.stdout.readline()
        ports=__import__('re').search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
        self.assertIsNotNone(ports,line)
        self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))

    def test_original_failed_trial_offer_diamond_purchase_use_free_reset_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        proof=json.loads((ROOT/'analysis/p7/maze-trader-prebuilt-proof.json').read_text())
        self.assertEqual(proof['status'],'PASS_DOMAIN_ALLPKG_VET_BUILD')
        self.source=Path(proof['source']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        for rel,sha in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),sha,rel)
        for rel,sha in proof['tableSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),sha,rel)
        prebuilt=Path(proof['binary']).resolve()
        self.assertEqual(prebuilt.parent,self.source)
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(),proof['binarySHA256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        # Check all original empty request/response fields before allocating a schema.
        self.assertEqual(list(message('CSRushMazeTrader').DESCRIPTOR.fields),[])
        self.assertEqual(list(message('CSRushMazeTraderBuy').DESCRIPTOR.fields),[])
        self.assertEqual(set(message('SCRushMazeTrader').DESCRIPTOR.fields_by_name),{'result','originalPrice','discountPrice'})
        self.assertEqual(set(message('SCRushMazeTraderBuy').DESCRIPTOR.fields_by_name),{'gain','cost'})
        original_raw=(ROOT/'analysis/p6/actual-local-p7-rush-terminal-inputs-v127/000.SCRushFightStart.pb').read_bytes()
        original=message('SCRushFightStart');original.ParseFromString(original_raw)
        self.assertEqual(len(original.battleMonsterList),1)
        enemy=original.battleMonsterList[0].SerializeToString(deterministic=True).hex()
        report=dict(status='PENDING',binarySHA256=proof['binarySHA256'],managedHelperSHA256=helper,originalLocal127StartSHA256=hashlib.sha256(original_raw).hexdigest(),finitePreparation={'PlayerLevel':22,'initialDiamond':100,'main':'unchanged actualCreate047level1/GUID/full3skills'},formalModified=False,phoneUI=False)
        began=time.monotonic();checkpoint=ROOT/'analysis/p7/maze-trader-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-maze-trader-native-')as temporary:
            self.binary=Path(temporary)/'maze-native';shutil.copy2(prebuilt,self.binary)
            self.native_log=Path(temporary)/'native.log'
            self.managed_pool=ManagedMySQL();allocation=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    allocation=self.managed_pool.allocate('p7_maze')
                    self.database,self.test_user=allocation.database,allocation.user
                    self.test_password=allocation.password
                    self.env=dict(os.environ,**allocation.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    for filename in ['174_p7_maze_trader_policies.sql','174_p7_maze_trader_policies.seed.sql','174_p7_maze_trader_offers.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/filename).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p7_maze_trader_policies;').strip(),'0')
                    self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME IN ('p7_maze_trader_policies','p7_maze_trader_offers','p7_rush_receipts','p6_battle_records','profiles');").strip(),'InnoDB')
                    identity='original-maze-'+secrets.token_hex(5);owner=self.login(identity);stamp=270000
                    def http(name,fields,expected):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,owner,stamp,expected)
                    def count(item):return sum(v['num']for v in(self.profile(owner)[1].get('items')or[])if v['id']==item)
                    http('CSPlayerCreate',{'playerName':'原迷宫商人','heroId':'hero_047'},'SCPlayerCreate')
                    initial=self.profile(owner)
                    self.assertEqual((initial[0].get('coin',0),initial[0].get('diamond',0),initial[0].get('energy',0)),(0,0,100))
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',22,'$.diamond',100) WHERE id={owner.playerId};")
                    http('CSFormationList',{},'SCFormationList')
                    stage='maze-local127-whole'
                    self.sql("INSERT INTO p7_rush_stage_rules(id,kind,task_id,condition_task_id,floor,star,min_level,enabled,scene,max_round,win_key,win_value,enemies,first_reward,reward,recommended_power,source) VALUES('"+stage+"',1,'"+stage+"','',1,1,22,1,'map_grass_001',40,0,0,0x"+enemy+",'','point,10',0,'existing prepublished local127 whole original-observed source; private floor unknown'); UPDATE p7_rush_policies SET enabled=1;UPDATE p7_rush_scoped_policies SET enabled=1;UPDATE p7_rush_manual_policies SET enabled=0;")
                    at=int(time.time()*1000)
                    self.sql(f'UPDATE p7_maze_trader_policies SET enabled=1,published_at={at-1000},start_at={at-1000},end_at={at+3600000};')
                    self.assertEqual(http('CSRushMazeTrader',{},'ErrorMessage').msg!='',True)
                    args={'rushType':1,'taskId':stage,'star':1}
                    start=http('CSRushFightStart',args,'SCRushFightStart')
                    self.assertEqual(start.battlePlayer.formation[0].guid,initial[0]['heroWarId'])
                    self.assertEqual(start.battlePlayer.formation[0].skillOrder,'047_1,047_2,047_3')
                    terminal=json.loads(bytes.fromhex(self.sql("SELECT HEX(terminal) FROM p7_rush_scoped_calculations WHERE fight_id='"+start.fightId+"';").strip()))
                    outcome=terminal['outcome'];actual=int(outcome['winner']==0)
                    self.assertEqual(actual,0,'Unexpected real result; preserve winner, do not change panel')
                    end={'fightId':start.fightId,'result':actual,'optRecord':terminal['originalOperates']}
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_maze_trader_offers;').strip(),'0')
                    before=self.profile(owner)
                    http('CSRushFightEnd',dict(end,result=1-actual),'ErrorMessage')
                    self.assertEqual(self.profile(owner),before)
                    ended=http('CSRushFightEnd',end,'SCRushFightEnd')
                    self.assertEqual(ended.result,0)
                    self.assertEqual(self.profile(owner)[0].get('point',0),0)
                    info=http('CSGetRushInfo',{},'SCGetRushInfo')
                    self.assertEqual((info.isMustReset,info.isMazeTraderValid),(1,1))
                    http('CSRushFightStart',args,'ErrorMessage')
                    self.assertEqual(self.call('CSRushMazeTrader',{},owner,0,'SCRushMazeTrader'),http('CSRushMazeTrader',{},'SCRushMazeTrader'))
                    held_no_buy=self.profile(owner)
                    for invalid in [0,-1]:self.assertEqual(self.stamped('CSRushMazeTraderBuy',{},owner,invalid)[0],BY_NAME['ErrorMessage'])
                    self.assertEqual(self.profile(owner),held_no_buy)
                    product=http('CSRushMazeTrader',{},'SCRushMazeTrader')
                    self.assertEqual((product.result.id,product.result.num,product.originalPrice,product.discountPrice),('item_001',2,20,10))
                    freeze=self.sql('SELECT CONCAT(policy_hash,"|",draw,"|",fight_id) FROM p7_maze_trader_offers;').strip()
                    self.sql('UPDATE p7_maze_trader_policies SET enabled=0,discount_price=999;')
                    before=self.profile(owner)
                    stamp+=1;buy_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:
                        replies=list(workers.map(lambda _:self.call('CSRushMazeTraderBuy',{},owner,buy_stamp,'SCRushMazeTraderBuy'),range(8)))
                    body=replies[0];self.assertTrue(all(v==body for v in replies))
                    self.assertEqual([(v.id,v.num)for v in body.cost],[('diamond',10)])
                    self.assertEqual(self.profile(owner)[0]['diamond'],before[0]['diamond']-10)
                    self.assertEqual(count('item_001'),2)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p7_rush_receipts WHERE domain="maze-buy";').strip(),'1')
                    http('CSRushMazeTraderBuy',{},'ErrorMessage')
                    consumed_before=self.profile(owner)[0]['energy']
                    http('CSItemUse',{'costItem':{'id':'item_001','num':2}},'SCItemUse')
                    self.assertEqual((count('item_001'),self.profile(owner)[0]['energy']),(0,consumed_before+20))
                    held=self.profile(owner)
                    self.stop();self.start();owner=self.login(identity)
                    self.assertEqual(self.call('CSRushMazeTraderBuy',{},owner,buy_stamp,'SCRushMazeTraderBuy'),body)
                    self.assertEqual(self.profile(owner),held)
                    self.assertEqual(self.sql('SELECT CONCAT(policy_hash,"|",draw,"|",fight_id) FROM p7_maze_trader_offers;').strip(),freeze)
                    info=http('CSGetRushInfo',{},'SCGetRushInfo');self.assertEqual((info.isMustReset,info.isMazeTraderValid),(1,0))
                    http('CSRushResetChallengeCount',{},'SCRushResetChallengeCount')
                    self.assertEqual(self.profile(owner)[0]['diamond'],held[0]['diamond'])
                    info=http('CSGetRushInfo',{},'SCGetRushInfo');self.assertEqual((info.isMustReset,info.isMazeTraderValid),(0,0))
                    http('CSRushMazeTrader',{},'ErrorMessage')
                    self.assertEqual(self.call('CSRushMazeTraderBuy',{},owner,buy_stamp,'SCRushMazeTraderBuy'),body)
                    self.assertEqual(self.sql('SELECT CONCAT(run_id,"|",free_reset_used) FROM p7_rush_states;').strip(),'2|1')
                    report.update(status='PASS',sourceFight=start.fightId,actualWinner=outcome['winner'],actualFinalRound=outcome['finalRound'],completeOriginalActions=len(terminal['originalOperates']['skillOpts']),independentProof=outcome['capabilityHash'],realCost10=True,actualItem001UseEnergy20=True,concurrentOnce8=True,closedPolicyFrozen=True,coldOnce=True,resetHidesOriginalMerchant=True)
                except BaseException as error:
                    report.update(status='FAIL',error=self.managed_pool.redact(repr(error)))
                    raise
                finally:
                    self.stop();self.managed_pool.close()
                    report.update(wallSeconds=round(time.monotonic()-began,3),resources={'process':0,'managedSchema':0,'managedAccount':0,'privateEnv':0})
                    checkpoint.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
