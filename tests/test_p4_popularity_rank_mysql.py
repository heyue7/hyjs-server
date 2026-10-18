"""Original Guide13 -> existing real accepted-win prize popularity -> original shop4 rank tab.

Opt-in coordinated single managed SQL window; reuse one precompiled binary.
Finite levels/cursor qualify the actual sources, with no currency/rank/winner
injection. The existing defaultoff local prize policy supplies actual popularity. No clock override is used.
"""
import concurrent.futures
import datetime
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

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P4_POPULARITY_RANK_MYSQL_TEST')=='1','explicit coordinated managed SQL window')
class P4PopularityRankMySQLTests(unittest.TestCase):
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

    def test_actual_win_popularity_original_rank_tab_once_use_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P4_POPULARITY_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        proof=json.loads((ROOT/'analysis/p4/popularity-rank-prebuilt-proof.json').read_text())
        self.assertEqual(proof['status'],'PASS_DOMAIN_ALLPKG_VET_BUILD')
        for rel,expected in proof['runtimeSHA256'].items():
            self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),expected,rel)
        prebuilt=Path(os.environ['HYJS_P4_POPULARITY_PREBUILT_BINARY']).resolve()
        self.assertEqual(prebuilt.parent,self.source)
        self.assertEqual(hashlib.sha256(prebuilt.read_bytes()).hexdigest(),proof['binarySHA256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'483b950cb684ae53716c403db0e541551c23dcce3932b073c8fd51ccf3e49da4')
        report=dict(status='PENDING',prebuiltProof=proof,managedHelperSHA256=helper,formalModified=False,phoneUI=False)
        began=time.monotonic()
        checkpoint=ROOT/'analysis/p4/popularity-rank-native-checkpoint.json'
        with tempfile.TemporaryDirectory(prefix='hyjs-popularity-native-')as temporary:
            self.binary=Path(temporary)/'popularity-native';shutil.copy2(prebuilt,self.binary)
            self.native_log=Path(temporary)/'native.log'
            self.managed_pool=ManagedMySQL();allocation=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    allocation=self.managed_pool.allocate('p11_popularity')
                    self.database,self.test_user=allocation.database,allocation.user
                    self.test_password=allocation.password
                    self.env=dict(os.environ,**allocation.env(),GOMEMLIMIT='768MiB',GOMAXPROCS='2')
                    for filename in ['83_p4_commerce_rules.sql','167_p4_popularity_rank_claims.sql','167_p4_popularity_rank_claims.seed.sql']:
                        self.sql((self.source/'hyjs-server/db/sql'/filename).read_text())
                    self.start()
                    self.assertEqual(self.sql("SELECT enabled FROM p4_commerce_rules WHERE kind='popularity-rank';").strip(),'0')
                    self.assertEqual(self.sql("SELECT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME='p4_popularity_rank_claims';").strip(),'InnoDB')
                    identities=['original-shop-rank-'+secrets.token_hex(4)for _ in range(3)]
                    peer,other,owner=[self.login(v)for v in identities];stamp=190000
                    def http(name,fields,actor,expected):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,actor,stamp,expected)
                    def count(item):return sum(v['num']for v in(self.profile(owner)[1].get('items')or[])if v['id']==item)
                    for i,actor in enumerate([peer,other,owner]):
                        http('CSPlayerCreate',{'playerName':'原声望用途'+str(i),'heroId':'hero_047'},actor,'SCPlayerCreate')
                        player,game=self.profile(actor)
                        self.assertEqual((player.get('coin',0),player.get('diamond',0),player.get('popularity',0)),(0,0,0))
                        self.assertEqual(next(v for v in game['warriors']if v['guid']==player['heroWarId'])['level'],1)
                        self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',{100 if actor==owner else 20},'$.noviceGuideSchedule',12) WHERE id={actor.playerId};")
                    off=http('CSShopProductList',{'shopType':4},owner,'SCShopProductList')
                    self.assertEqual({v.id for v in off.shopItemList},{'p4Popularity_1','p4Popularity_2'})
                    self.assertTrue(all(not v.HasField('condition')and v.canBuy==1 and v.dayLeftTimes==20 for v in off.shopItemList))
                    local=(datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))+datetime.timedelta(seconds=3)).replace(microsecond=0)
                    start=int(local.timestamp()*1000);end=start+86400000;published=int(time.time()*1000)
                    month_end=(local.replace(day=28)+datetime.timedelta(days=4)).replace(day=1,hour=0,minute=0,second=0,microsecond=0)
                    pool=json.dumps([{'id':'popularity','num':60,'weight':1}],separators=(',',':'))
                    self.sql(f"UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;UPDATE p11_arena_prize_policies SET enabled=1,version='local-guide-popularity-source-v1',published_at={published},start_at={start},end_at={int(month_end.timestamp()*1000)},min_level=10,daily_wins=1,window_wins=1,pool_json=CONVERT(0x{pool.encode().hex()} USING utf8mb4);UPDATE p4_commerce_rules SET enabled=1,start_at={start},end_at={start+3600000},claim_end_at={start+3600000},rules=JSON_SET(CONVERT(rules USING utf8mb4),'$.rankPublishedAt',{published}) WHERE kind='popularity-rank';")
                    remaining=start/1000-time.time()
                    if remaining>0:time.sleep(remaining+0.05)
                    initial=http('CSShopProductList',{'shopType':4},owner,'SCShopProductList')
                    sku=next(v for v in initial.shopItemList if v.id.startswith('p4PopularityRank_'))
                    self.assertEqual((sku.condition.type,sku.condition.value,sku.canBuy),('rank',1000,0))
                    args={'shopType':4,'shopItemId':sku.id,'buyCount':1}
                    stamp+=1;self.assertEqual(self.stamped('CSShopBuy',args,owner,stamp)[0],10001)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_popularity_rank_claims;').strip(),'0')
                    targets=http('CSPvpFindMatchGuide',{},owner,'SCPvpFindMatchGuide')
                    self.assertEqual({v.info.playerId for v in targets.rankList},{peer.playerId,other.playerId})
                    energy=self.profile(owner)[0]['energy']
                    fight=http('CSPvpFightResultGuide',{'playerId':peer.playerId,'serverId':1},owner,'SCPvpFightResultGuide')
                    self.assertEqual(fight.winPlayerId,owner.playerId)
                    self.assertEqual(self.profile(owner)[0]['energy'],energy-2)
                    self.assertEqual(self.profile(owner)[0]['noviceGuideSchedule'],13)
                    # Original three-card prize grants only card0. A named local
                    # pool of60 supplies exactly60 actual popularity, never180.
                    self.assertEqual([(v.id,v.num)for v in fight.boxList],[('popularity',60)]*3)
                    self.assertEqual(sum(v.num for v in fight.result.simps if v.id=='popularity'),60)
                    self.assertEqual(self.profile(owner)[0]['popularity'],60)
                    self.assertEqual(self.sql("SELECT COUNT(*) FROM p11_arena_prize_awards WHERE status='awarded';").strip(),'1')
                    offered=http('CSShopProductList',{'shopType':4},owner,'SCShopProductList')
                    self.assertEqual(next(v for v in offered.shopItemList if v.id==sku.id).canBuy,1)
                    frozen=self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE kind='popularity-rank';")
                    self.sql("UPDATE p4_commerce_rules SET rules=JSON_SET(CONVERT(rules USING utf8mb4),'$.products[0].cost','popularity,4999','$.products[0].maxRank',1) WHERE kind='popularity-rank';")
                    stamp+=1;buy_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:
                        buys=list(workers.map(lambda _:self.call('CSShopBuy',args,owner,buy_stamp,'SCShopBuy'),range(8)))
                    bought=buys[0];self.assertTrue(all(v==bought for v in buys))
                    self.assertEqual([(v.id,v.num)for v in bought.cost],[('popularity',20)])
                    self.assertEqual(next(v for v in bought.shopItemList if v.id==sku.id).canBuy,2)
                    self.assertEqual((self.profile(owner)[0]['popularity'],self.profile(owner)[0]['noviceGuideSchedule'],count('item_001')),(40,14,1))
                    public=http('CSPlayer',{},owner,'SCPlayer').player
                    self.assertEqual((public.popularity,public.noviceGuideSchedule),(40,14))
                    stamp+=1;self.assertEqual(self.stamped('CSShopBuy',args,owner,stamp)[0],10001)
                    normal=http('CSShopBuy',{'shopType':4,'shopItemId':'p4Popularity_1','buyCount':2},owner,'SCShopBuy')
                    self.assertEqual([(v.id,v.num)for v in normal.cost],[('popularity',40)])
                    ordinary=next(v for v in normal.shopItemList if v.id=='p4Popularity_1')
                    self.assertFalse(ordinary.HasField('condition'));self.assertEqual((ordinary.canBuy,ordinary.dayLeftTimes),(1,18))
                    energy=self.profile(owner)[0]['energy']
                    http('CSItemUse',{'costItem':{'id':'item_001','num':3}},owner,'SCItemUse')
                    self.assertEqual((count('item_001'),self.profile(owner)[0]['energy']),(0,energy+30))
                    stable=self.profile(owner)
                    self.assertEqual(stable[0].get('popularity',0),0)
                    claim=self.sql('SELECT `rank`,LENGTH(rank_proof),LENGTH(offer_hash) FROM p4_popularity_rank_claims;').strip()
                    self.assertEqual(claim,'1\t64\t64')
                    self.sql("UPDATE p4_commerce_rules SET enabled=0 WHERE kind='popularity-rank';")
                    self.stop();self.start();owner=self.login(identities[2])
                    self.assertEqual(self.call('CSShopBuy',args,owner,buy_stamp,'SCShopBuy'),bought)
                    self.assertEqual(self.profile(owner),stable)
                    self.assertEqual(self.sql("SELECT HEX(frozen) FROM p4_commerce_rules WHERE kind='popularity-rank';"),frozen)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p4_popularity_rank_claims;').strip(),'1')
                    report.update(status='PASS_ORIGINAL_GUIDE13_WIN_PRIZE_POPULARITY_RANK_SHOP_GUIDE14_USE_COLD',actualWinner=fight.winPlayerId,actualWinPopularity=60,actualRankCost=20,actualOrdinaryBatchCost=40,actualUsed001=3,actualEnergyGain=30,currentPopularity=0,currentGuide=14,sameBuyWorkers=8,finitePrerequisites='Original Create047×3, finite Playerlevels100/20/20 and prior guidecursor12 only. Main1/GUID/full3skills/wallet0 preserved. Actual Guide battle winner/rank and the existing prepublished local win-prize supply all60 popularity; no clock override.',sourceBoundary='Actual win-prize60 and rank-tab SKU/rank1000/cost20/once are explicit prepublished local defaultoff terms; original private catalogue unknown. Existing2ordinary IDs/prices/quota20 unchanged.')
                except BaseException as error:
                    report.update(status='FAIL',failure=self.managed_pool.redact(str(error)));raise
                finally:
                    try:self.stop()
                    finally:
                        if allocation is not None:self.managed_pool.release(allocation.database)
                        self.managed_pool.close()
                    report.update(managedAllocationsRemoved=True,seconds=round(time.monotonic()-began,3))
                    checkpoint.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
        (ROOT/'analysis/p4/popularity-rank-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
