"""Actual accepted challenger -> daily cutoff/P1 -> original shop/training.

Opt-in single managed SQL window. Original seven tiers are unchanged. The
unknown cutoff/participation schedule is a prepublished local default-off policy.
Only the isolated daily deadline is advanced; real P6 winner/rank and resources
come from the original APIs and are never inserted by this runner.
"""
import concurrent.futures
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
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

@unittest.skipUnless(os.environ.get('HYJS_P11_ARENA_DAILY_MYSQL_TEST')=='1','explicit single managed SQL window')
class P11ArenaDailyMySQLTests(unittest.TestCase):
    send=Transport.send
    login=Transport.login
    stamped=Transport.stamped
    call=Transport.call
    profile=Transport.profile
    stop=Transport.stop
    setUp=Transport.setUp
    sql=Transport.sql

    def start(self):
        self.process=subprocess.Popen([str(self.binary),'--http-port','0','--tcp-port','0','--tables',str(self.source/'analysis/tables'),'--log-file',str(self.clock_path.parent/'native.log')],cwd=self.source/'hyjs-server',env=self.env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True)
        with selectors.DefaultSelector()as ready:
            ready.register(self.process.stdout,selectors.EVENT_READ)
            self.assertTrue(ready.select(90),'daily native startup timeout')
            line=self.process.stdout.readline()
        ports=re.search(r'HTTP=127.0.0.1:(\d+) TCP=127.0.0.1:(\d+)',line)
        self.assertIsNotNone(ports,line)
        self.service=types.SimpleNamespace(http_port=int(ports[1]),tcp_port=int(ports[2]))

    def test_original_daily_rank_p1_offline_shop_training_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        source=Path(os.environ['HYJS_P11_ARENA_DAILY_BUILD_ROOT']).resolve()
        self.assertEqual(source.parent,ROOT/'build/candidates')
        owned=json.loads((ROOT/'analysis/p11/arena-daily-code-manifest.json').read_text())['files']
        shared=json.loads((ROOT/'analysis/p11/arena-daily-shared-proposal-20261015/manifest.json').read_text())['files']
        paths=[v['path']for v in owned]+['hyjs-server/'+v['path']for v in shared]
        hashes={v:hashlib.sha256((source/v).read_bytes()).hexdigest()for v in paths}
        for v in owned:self.assertEqual(hashes[v['path']],v['sha256'])
        for v in shared:self.assertEqual(hashes['hyjs-server/'+v['path']],v['sha256'])
        helper=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper,'ec02759e488a72c40b08c2da629d7d2be2adcca26b5ce0f056f7b04135ad56bd')
        report={'status':'PENDING','sourceSHA256':hashes,'managedHelperSHA256':helper,'productionModified':False,'phoneUI':False}
        began=time.monotonic()
        with tempfile.TemporaryDirectory(prefix='hyjs-arena-daily-')as temporary:
            temporary=Path(temporary);self.source=temporary/'source';self.source.mkdir()
            shutil.copytree(source/'hyjs-server',self.source/'hyjs-server')
            (self.source/'analysis').symlink_to((source/'analysis').resolve(),target_is_directory=True)
            self.clock_path=temporary/'daily-clock.txt';self.clock_path.write_text('0')
            main=self.source/'hyjs-server/main.go';text=main.read_text()
            text=text.replace('"hyjs-server/config"','"hyjs-server/config"\n "hyjs-server/constUtil"\n "strings"',1)
            needle='\tdefer cancel()\n'
            self.assertEqual(text.count(needle),1)
            # This hook exists only in this disposable native source. It refuses
            # all schemas outside this runner's managed prefix and changes only
            # the new daily cutoff context, with normal actual time before close.
            hook='\n if clockPath:=os.Getenv("HYJS_TEST_ARENA_DAILY_CLOCK");clockPath!="" {\n  if cfg.Driver!="mysql"||!strings.HasPrefix(config.AppConfigIns.MysqlConfig.DbName,"hyjs_test_p11_daily_"){return errors.New("isolated daily clock schema guard")}\n  ctx=context.WithValue(ctx,constUtil.P11ArenaDailyClockContextKey{},func()time.Time{\n   raw,e:=os.ReadFile(clockPath);if e!=nil{panic(e)}\n   value,e:=strconv.ParseInt(strings.TrimSpace(string(raw)),10,64);if e!=nil{panic(e)}\n   if value==0{return time.Now()};return time.UnixMilli(value)\n  })\n }\n'
            main.write_text(text.replace(needle,needle+hook,1))
            self.binary=temporary/'daily-native'
            build=subprocess.run(['rtk','proxy','go','build','-o',str(self.binary),'.'],cwd=self.source/'hyjs-server',capture_output=True,text=True,timeout=240)
            self.assertEqual(build.returncode,0,(build.stdout+build.stderr)[-4000:])
            report['guardedTestMainSHA256']=hashlib.sha256(main.read_bytes()).hexdigest()
            self.managed_pool=ManagedMySQL();connection=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    connection=self.managed_pool.allocate('p11_daily')
                    self.database=connection.database;self.test_user=connection.user;self.test_password=connection.password
                    self.assertTrue(self.test_user.startswith('hyjs_t_'))
                    self.env=dict(os.environ,**connection.env(),GOMEMLIMIT='1GiB',HYJS_TEST_ARENA_DAILY_CLOCK=str(self.clock_path))
                    for name in ['policies.sql','offers.sql','snapshots.sql','awards.sql','challenges.sql','policies.seed.sql']:
                        self.sql((source/'hyjs-server/db/sql'/('149_p11_arena_daily_'+name)).read_text())
                    self.start()
                    self.assertEqual(self.sql('SELECT enabled FROM p11_arena_daily_policies WHERE id=1;').strip(),'0')
                    self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p11_arena_daily_%';").strip(),'InnoDB')
                    names=['daily-original-'+secrets.token_hex(4)for _ in range(2)]
                    defender,owner=[self.login(v)for v in names]
                    for login,label in [(defender,'原每日榜对手'),(owner,'原每日榜用途')]:self.call('CSPlayerCreate',{'playerName':label,'heroId':'hero_047'},login,1,'SCPlayerCreate')
                    # Finite levels qualify the original source. No rank, winner,
                    # money, training material or reward attachment is injected.
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id={defender.playerId};UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id={owner.playerId};UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;")
                    stamp=90000
                    def http(name,fields,login,expected):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,expected)
                    def count(item):return sum(v['num']for v in(self.profile(owner)[1].get('items')or[])if v['id']==item)
                    local=datetime.datetime.now(datetime.timezone(datetime.timedelta(hours=8)))+datetime.timedelta(seconds=3)
                    local=local.replace(microsecond=0);start=int(local.timestamp()*1000);end=start+86400000
                    cutoff=local.hour*3600+local.minute*60+local.second
                    self.sql(f"UPDATE p11_arena_daily_policies SET enabled=1,version='local-native-daily-seven-tiers-v1',published_at={int(time.time()*1000)},start_at={start},end_at={end},cutoff_second={cutoff},min_challenges=1 WHERE id=1;")
                    remaining=start/1000-time.time()
                    if remaining>0:time.sleep(remaining+0.05)
                    http('CSPvpFindMatch',{},owner,'SCPvpFindMatch')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_daily_offers;').strip(),'1')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_daily_awards;').strip(),'0')
                    before=self.profile(owner);args={'playerId':defender.playerId,'serverId':1};stamp+=1;fight_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:
                        replies=list(workers.map(lambda _:self.call('CSPvpFightResult',args,owner,fight_stamp,'SCPvpFightResult'),range(8)))
                    result=replies[0];self.assertTrue(all(v==result for v in replies));self.assertEqual(result.winPlayerId,owner.playerId)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_daily_challenges;').strip(),'1')
                    self.assertEqual(self.profile(owner)[0]['energy'],before[0]['energy']-2)
                    frozen=self.sql('SELECT source_hash,rules_json FROM p11_arena_daily_offers;')
                    before_delivery=self.profile(owner)
                    self.sql("UPDATE p11_arena_daily_policies SET enabled=0,min_challenges=100,version='current-policy-changed-after-offer';")
                    self.stop();self.clock_path.write_text(str(end+1));self.start()
                    owner=self.login(names[1])
                    self.assertEqual(self.sql('SELECT source_hash,rules_json FROM p11_arena_daily_offers;'),frozen)
                    self.assertEqual(self.sql('SELECT status FROM p11_arena_daily_snapshots;').strip(),'complete')
                    award=self.sql('SELECT player_id,position,rewards,delivery_id FROM p11_arena_daily_awards;').strip().split('\t')
                    self.assertEqual(award[:3],[str(owner.playerId),'1','popularity,5000;coin,120000;item_007,50']);delivery=int(award[3])
                    self.assertEqual(self.profile(owner),before_delivery)
                    pending=http('CSQueryUserReward',{},owner,'SCQueryUserReward')
                    self.assertEqual(len([v for v in pending.rewardItemInfoList if v.rewardId==delivery]),1)
                    claim={'rewardIds':[delivery]}
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as workers:
                        claims=list(workers.map(lambda _:http('CSAddRewardsToPlayer',claim,owner,'SCAddRewardsToPlayer'),range(8)))
                    gained={key:sum(x.num for r in claims for x in r.result.simps if x.id==key)for key in ['coin','popularity','item_007']}
                    self.assertEqual(gained,{'coin':120000,'popularity':5000,'item_007':50})
                    self.assertEqual(count('item_007'),50)
                    self.assertEqual(self.profile(owner)[0]['coin'],before_delivery[0]['coin']+120000)
                    self.assertEqual(self.profile(owner)[0]['popularity'],before_delivery[0].get('popularity',0)+5000)
                    # Arena UI tab7 maps to original message shopType4. Its
                    # existing published product/price is reused unchanged.
                    listing=http('CSShopProductList',{'shopType':4},owner,'SCShopProductList')
                    sku=next(v for v in listing.shopItemList if v.id=='p4Popularity_1')
                    self.assertEqual([(v.id,v.num)for v in sku.exchangeList],[('popularity',20)])
                    bought=http('CSShopBuy',{'shopType':4,'shopItemId':sku.id,'buyCount':1},owner,'SCShopBuy')
                    self.assertEqual([(v.id,v.num)for v in bought.cost],[('popularity',20)])
                    self.assertEqual(self.profile(owner)[0]['popularity'],before_delivery[0].get('popularity',0)+4980)
                    self.assertEqual(count('item_001'),1)
                    energy=self.profile(owner)[0]['energy']
                    http('CSItemUse',{'costItem':{'id':'item_001','num':1}},owner,'SCItemUse')
                    self.assertEqual(count('item_001'),0);self.assertEqual(self.profile(owner)[0]['energy'],energy+10)
                    main_guid=self.profile(owner)[0]['heroWarId']
                    training=http('CSWarriorTraining',{'guid':main_guid,'costType':1,'numType':1},owner,'SCWarriorTraining')
                    self.assertEqual([(v.id,v.num)for v in training.cost],[('item_007',5)]);self.assertEqual(count('item_007'),45)
                    replaced=False
                    if training.guid.tempTraining.state==1:
                        http('CSWarriorTrainingReplace',{'guid':main_guid,'result':1},owner,'SCWarriorTrainingReplace');replaced=True
                    stable=self.profile(owner)
                    self.stop();self.start();owner=self.login(names[1])
                    self.assertEqual(self.profile(owner),stable)
                    self.assertEqual(self.call('CSPvpFightResult',args,owner,fight_stamp,'SCPvpFightResult'),result)
                    again=http('CSAddRewardsToPlayer',claim,owner,'SCAddRewardsToPlayer');self.assertEqual(len(again.result.simps),0)
                    self.assertEqual(self.profile(owner),stable)
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_daily_awards;').strip(),'1')
                    self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_daily_challenges;').strip(),'1')
                    report.update(status='PASS_ORIGINAL_DAILY_RANK_P1_OFFLINE_SHOP_TRAINING_COLD',finitePrerequisites='Original CreatePlayer047 plus finite isolated levels100/20. Local schedule prepublished before future real start; only daily deadline advanced after real accepted challenge. No winner/rank/currency/item/GUID injection.',actualReward=gained,actualRank=1,actualEnergyPayment=2,actualShopPopularityCost=20,actualTraining007Cost=5,actualTrainingReplaced=replaced,sameFightWorkers=8,sameP1ClaimWorkers=8,coldRestarts=2,applicationAndFixtureTemporaryAccount=True)
                finally:
                    try:self.stop()
                    finally:
                        try:
                            if connection is not None:self.managed_pool.release(connection.database)
                        finally:self.managed_pool.close()
                    report['managedAllocationsRemoved']=True
        for p,digest in hashes.items():self.assertEqual(hashlib.sha256((source/p).read_bytes()).hexdigest(),digest)
        report['seconds']=round(time.monotonic()-began,3)
        (ROOT/'analysis/p11/arena-daily-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
