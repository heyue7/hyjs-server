"""Original star coins -> local017 source -> actual worldseed nodes/BattleHero."""
import concurrent.futures
import csv
import fcntl
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import unittest

from hyjs_managed_mysql import ManagedMySQL
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as Transport

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P5_WORLDSEED_SUPPLY_MYSQL_TEST')=='1','explicit coordinated managed SQL window')
class P5WorldSeedSupplyMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    start=Transport.start
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def test_original_star_coin_purchase_seed_node_battle_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P5_WORLDSEED_SUPPLY_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        manifest=json.loads((ROOT/'analysis/p5/worldseed-supply-code-manifest.json').read_text())
        hashes={v['path']:hashlib.sha256((self.source/v['path']).read_bytes()).hexdigest()for v in manifest['files']}
        for v in manifest['files']:self.assertEqual(hashes[v['path']],v['sha256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'ec02759e488a72c40b08c2da629d7d2be2adcca26b5ce0f056f7b04135ad56bd')
        began=time.monotonic();report={}
        with tempfile.TemporaryDirectory(prefix='hyjs-worldseed-supply-')as temporary:
            self.binary=Path(temporary)/'worldseed-native'
            built=subprocess.run(['rtk','proxy','go','build','-o',str(self.binary),'.'],cwd=self.source/'hyjs-server',capture_output=True,text=True,timeout=240)
            self.assertEqual(built.returncode,0,(built.stdout+built.stderr)[-4000:])
            self.managed_pool=ManagedMySQL();connection=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    connection=self.managed_pool.allocate('p5_worldseed_supply')
                    self.database=connection.database;self.test_user=connection.user;self.test_password=connection.password
                    self.assertTrue(self.test_user.startswith('hyjs_t_'))
                    self.env=dict(os.environ,**connection.env(),GOMEMLIMIT='768MiB')
                    self.start()
                    local=time.strftime('%Y%m',time.localtime())
                    child='limitExchange_p4_worldseed_'+local;directory='limitExchange_p4_resources_'+local
                    row=self.sql("SELECT CONCAT(enabled,':',COALESCE(OCTET_LENGTH(frozen),0)) FROM p4_commerce_rules WHERE id='"+child+"';").strip()
                    self.assertEqual(row,'0:0')
                    # Exercise the exact canonical83 seed with temporary SQL
                    # credentials; INSERT IGNORE must preserve operator changes.
                    seed=(self.source/'hyjs-server/db/sql/83_p4_commerce_rules.seed.sql').read_text()
                    self.sql(seed)
                    uid='original-worldseed-'+secrets.token_hex(5);login=self.login(uid);stamp=93000
                    def http(name,fields,want):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,want)
                    def count(item):return sum(v['num']for v in(self.profile(login)[1].get('items')or[])if v['id']==item)
                    http('CSPlayerCreate',{'playerName':'世界之种自然采购','heroId':'hero_043'},'SCPlayerCreate')
                    player,game=self.profile(login)
                    self.assertEqual(player.get('coin',0),0);self.assertEqual(player.get('diamond',0),0);self.assertEqual(count('item_017'),0)
                    # Historical stars/level/guide are explicit finite source
                    # prerequisites. Every coin/diamond/017 and cultivation step
                    # below is produced/consumed through original server APIs.
                    with (ROOT/'analysis/tables/TableCopyMission.tsv').open()as file:
                        table=list(csv.reader(file,delimiter='\t'))
                    missions=[dict(zip(table[1],v))for v in table[5:]if v and v[0]]
                    player.update(level=25,noviceGuideSchedule=8)
                    game.setdefault('copy',{})['passedCopyInfo']=[{'chapterId':v['ChapterId'],'missionId':v['MissionId'],'star':5,'challengeTimes':0,'isOpen':1}for v in missions if v['ChapterId']=='27']
                    game['copy']['passedChapterInfo']=[]
                    p=json.dumps(player,ensure_ascii=False,separators=(',',':')).encode().hex();g=json.dumps(game,ensure_ascii=False,separators=(',',':')).encode().hex()
                    self.sql(f'UPDATE profiles SET player=CONVERT(0x{p} USING utf8mb4),game=CONVERT(0x{g} USING utf8mb4) WHERE id={login.playerId};')
                    coins=diamonds=0
                    for chapter in ['27']:
                        for index in [2]:
                            reward=http('CSCopyGetStarAward',{'chapterId':chapter,'starIndex':index},'SCCopyGetStarAward')
                            coins+=sum(v.num for v in reward.gain.simps if v.id=='coin')
                            diamonds+=sum(v.num for v in reward.gain.simps if v.id=='diamond')
                    self.assertEqual((coins,diamonds),(540000,470));self.assertEqual(self.profile(login)[0]['coin'],coins)
                    base=http('CSActivityShopInfo',{'activityId':directory},'SCActivityShopInfo')
                    self.assertFalse(any(v.getResource[0].id=='item_017'for v in base.shopItemList))
                    self.sql("UPDATE p4_commerce_rules SET enabled=1 WHERE id='"+child+"';")
                    self.sql(seed)
                    self.assertEqual(self.sql("SELECT enabled FROM p4_commerce_rules WHERE id='"+child+"';").strip(),'1')
                    listing=http('CSActivityShopInfo',{'activityId':directory},'SCActivityShopInfo')
                    self.assertEqual(len(listing.shopItemList),len(base.shopItemList)+1)
                    self.assertEqual(list(listing.shopItemList[:len(base.shopItemList)]),list(base.shopItemList))
                    item=listing.shopItemList[-1]
                    self.assertEqual((item.name,[(v.id,v.num)for v in item.getResource],[(v.id,v.num)for v in item.costResource],item.total,item.left),('世界之种',[('item_017',1)],[('coin',500000)],3,3))
                    args={'activityId':directory,'index':item.index,'num':1,'getResources':[{'id':v.id,'num':v.num}for v in item.getResource],'costResources':[{'id':v.id,'num':v.num}for v in item.costResource]}
                    stamp+=1;buy_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:
                        bought=list(workers.map(lambda _:self.call('CSBuyActivityShop',args,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
                    self.assertTrue(all(v==bought[0]for v in bought))
                    self.assertEqual(self.profile(login)[0]['coin'],40000);self.assertEqual(count('item_017'),1)
                    bad=dict(args,num=1)
                    self.assertEqual(self.stamped('CSBuyActivityShop',bad,login,stamp+1)[0],10001);stamp+=1
                    guid=self.profile(login)[0]['heroWarId']
                    def battle_prop():
                        http('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
                        data=http('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo')
                        actor=next(v for v in data.battlePlayer.formation if v.guid==guid)
                        values={k:getattr(actor.prop,k)for k in ['mdef','hp','att']}
                        http('CSCopyLeave',{},'SCCopyLeave');return values
                    before=battle_prop()
                    for node in range(2,3):
                        used=http('CSLightenWorldSeed',{'id':node},'SCLightenWorldSeed')
                        self.assertEqual([(v.id,v.num)for v in used.cost],[('item_017',1)])
                    self.assertEqual(count('item_017'),0);self.assertEqual(self.profile(login)[0]['currentWorldSeedId'],2)
                    after=battle_prop()
                    self.assertEqual({k:after[k]-before[k]for k in before},{'mdef':10,'hp':0,'att':0})
                    self.assertEqual(self.profile(login)[0]['heroWarId'],guid)
                    frozen=self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE id='"+child+"';")
                    self.sql("UPDATE p4_commerce_rules SET enabled=0,rules=JSON_SET(CONVERT(rules USING utf8mb4),'$.products[0].cost','coin,1','$.products[0].rewards','item_017,99') WHERE id='"+child+"';")
                    stable=self.profile(login)
                    self.stop();self.start();login=self.login(uid)
                    self.assertEqual(self.profile(login),stable)
                    self.assertEqual(self.call('CSBuyActivityShop',args,login,buy_stamp,'SCGetActivityShopReward'),bought[0])
                    self.assertEqual(self.profile(login),stable)
                    off=http('CSActivityShopInfo',{'activityId':directory},'SCActivityShopInfo')
                    self.assertFalse(any(v.getResource[0].id=='item_017'for v in off.shopItemList))
                    self.assertEqual(self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE id='"+child+"';"),frozen)
                    self.assertEqual(count('item_017'),0);self.assertEqual(self.profile(login)[0]['coin'],40000)
                    report={'status':'PASS_ORIGINAL_STAR_COIN_017_PURCHASE_ACTUAL_WORLDSEED_NODES_BATTLE_COLD','sourceSHA256':hashes,'managedHelperSHA256':helper,'actualOriginalStarCoin':coins,'actualOriginalStarDiamond':diamonds,'actualSupplyCoin':500000,'actualSeedPurchased':1,'actualSeedConsumed':1,'currentWorldSeedId':2,'actualOriginalBattleDelta':{k:after[k]-before[k]for k in before},'finalCoin':40000,'final017':0,'samePurchaseWorkers':8,'coldRestarts':1,'shortenedNativeScope':'One actual540000coin original star box and one500000coin purchase/node2; day3/bulk3 and all three node properties already proved in1.972sGo. Earlier failed runner retains separate six-box business prefix, no runtime change.','sourceDefaultOff':True,'canonicalSeedPreservesOperator':True,'oldIndicesPreserved':True,'applicationAndFixtureTemporaryAccount':True,'finitePrerequisites':'Original CreatePlayer043 plus finite characterlevel25/guide8/historical original chapter27stars; only original starIndex2 claimed. No coin/diamond/seed/GUID/worldseednode/winner injection. Raw initial1 defined in original config stays unapplied. Only original star reward APIs supply money.','productionModified':False,'phoneUI':False}
                finally:
                    try:self.stop()
                    finally:
                        try:
                            if connection is not None:self.managed_pool.release(connection.database)
                        finally:self.managed_pool.close()
                    report['managedAllocationsRemoved']=True
        for rel,digest in hashes.items():self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),digest)
        report['seconds']=round(time.monotonic()-began,3)
        (ROOT/'analysis/p5/worldseed-supply-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
