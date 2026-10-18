"""Verified modern Arena2 win -> original Type9 -> P1 claim/use/titles/cold."""
import concurrent.futures
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
from test_p11_arena_prize_mysql import P11ArenaPrizeMySQLTests as PrizeTransport

ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P4_ARENA_WIN_MYSQL_TEST')=='1','explicit single managed SQL window')
class P4ArenaWinMySQLTests(unittest.TestCase):
    send=PrizeTransport.send
    login=PrizeTransport.login
    stamped=PrizeTransport.stamped
    call=PrizeTransport.call
    profile=PrizeTransport.profile
    start=PrizeTransport.start
    stop=PrizeTransport.stop
    setUp=PrizeTransport.setUp
    sql=PrizeTransport.sql

    def test_real_win_original_type9_claim_use_titles_and_cold(self):
        self.assertEqual(os.environ.get('HYJS_LOCAL_SERVICES_READY'),'1')
        self.source=Path(os.environ['HYJS_P4_ARENA_WIN_BUILD_ROOT']).resolve()
        self.assertEqual(self.source.parent,ROOT/'build/candidates')
        manifest=json.loads((ROOT/'analysis/p4/arena-win-code-manifest.json').read_text())
        source_hashes={v['path']:hashlib.sha256((self.source/v['path']).read_bytes()).hexdigest()for v in manifest['owned']}
        for v in manifest['owned']:self.assertEqual(source_hashes[v['path']],v['sha256'])
        shared='hyjs-server/service/periodService.go'
        source_hashes[shared]=hashlib.sha256((self.source/shared).read_bytes()).hexdigest()
        helper_sha=hashlib.sha256((ROOT/'tests/hyjs_managed_mysql.py').read_bytes()).hexdigest()
        self.assertEqual(helper_sha,'ec02759e488a72c40b08c2da629d7d2be2adcca26b5ce0f056f7b04135ad56bd')
        began=time.monotonic();report={}
        with tempfile.TemporaryDirectory(prefix='hyjs-p4-arena-win-')as temporary:
            self.binary=Path(temporary)/'native'
            built=subprocess.run(['rtk','proxy','go','build','-o',str(self.binary),'.'],cwd=self.source/'hyjs-server',capture_output=True,text=True,timeout=240)
            self.assertEqual(built.returncode,0,(built.stdout+built.stderr)[-4000:])
            self.managed_pool=ManagedMySQL();connection=None
            with open('/tmp/hyjs-isolated-mysql.lock','a+')as lock:
                fcntl.flock(lock,fcntl.LOCK_EX)
                try:
                    connection=self.managed_pool.allocate('p4_arena_win')
                    self.database=connection.database;self.test_user=connection.user;self.test_password=connection.password
                    self.env=dict(os.environ,**connection.env());self.env['GOMEMLIMIT']='1GiB'
                    self.start()
                    cfg=self.sql("SELECT id,enabled FROM p1_period_instances WHERE kind='pvp_win';").strip().split('\t')
                    self.assertEqual(len(cfg),2);key=cfg[0];self.assertEqual(cfg[1],'0')
                    self.assertTrue(key.startswith('pvpWinTimes_p4_arena_'))
                    names=['p4-real-arena-'+secrets.token_hex(4)for _ in range(2)]
                    defender,owner=[self.login(name)for name in names]
                    for login,label in [(defender,'活动真实对手'),(owner,'竞技实际福利')]:
                        self.call('CSPlayerCreate',{'playerName':label,'heroId':'hero_047'},login,1,'SCPlayerCreate')
                    # Finite level qualification only. All assets, owned GUIDs,
                    # winner, rank, facts, activity progress and titles are real.
                    self.sql(f"UPDATE profiles SET player=JSON_SET(player,'$.level',20) WHERE id={defender.playerId};UPDATE profiles SET player=JSON_SET(player,'$.level',100) WHERE id={owner.playerId};UPDATE p11_arena_policies SET enabled=1;UPDATE p11_arena_fight_policies SET enabled=1;")
                    stamp=300
                    def http(name,fields,login,expected):
                        nonlocal stamp
                        stamp+=1;return self.call(name,fields,login,stamp,expected)
                    def facts(login):
                        return int(self.sql(f"SELECT COUNT(*) FROM p1_event_facts WHERE player_id={login.playerId} AND kind='pvp_win';").strip())
                    def count(item):
                        return sum(v['num']for v in(self.profile(owner)[1].get('items')or[])if v['id']==item)
                    http('CSPvpFindMatch',{},owner,'SCPvpFindMatch')
                    self.assertEqual([facts(v)for v in[owner,defender]],[0,0])
                    before=self.profile(owner);args={'playerId':defender.playerId,'serverId':1};stamp+=1;fight_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:
                        won=list(pool.map(lambda _:self.call('CSPvpFightResult',args,owner,fight_stamp,'SCPvpFightResult'),range(8)))
                    result=won[0];self.assertTrue(all(v==result for v in won));self.assertEqual(result.winPlayerId,owner.playerId)
                    self.assertEqual(self.profile(owner)[0]['energy'],before[0]['energy']-2)
                    self.assertEqual([facts(v)for v in[owner,defender]],[1,0]);self.assertEqual(count('item_027'),0)
                    self.assertEqual(self.sql(f"SELECT source_key FROM p1_event_facts WHERE player_id={owner.playerId} AND kind='pvp_win';").strip(),'arena-win:'+result.fightId)
                    # Publish the named local one-win option after the real fact.
                    # Existing open-window reconciliation consumes only its
                    # accepted P11/P6 witness and creates no past battle rows.
                    self.sql(f"UPDATE p1_period_instances SET enabled=1,goal=1 WHERE id='{key}';")
                    tabs=http('CSActivityList',{},owner,'SCActivityList')
                    matching=[v for v in tabs.activityList if v.activityId.startswith('pvpWinTimes')]
                    self.assertEqual([v.activityId for v in matching],[key])
                    info=http('CSActivityInfo',{'activityId':key},owner,'SCActivityInfo')
                    self.assertEqual(len(info.itemList),1);item=info.itemList[0]
                    self.assertEqual((item.value,item.left,item.total,item.status),('1',1,1,1))
                    self.assertEqual([(v.id,v.num)for v in item.rewardList],[('item_027',1)])
                    request={'activityId':key,'index':1,'getResources':[{'id':v.id,'num':v.num}for v in item.rewardList]}
                    stamp+=1;claim_stamp=stamp
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8)as pool:
                        claims=list(pool.map(lambda _:self.call('CSActivityReward',request,owner,claim_stamp,'SCActivityReward'),range(8)))
                    self.assertEqual(sum(v.num for claim in claims for v in claim.get.simps if v.id=='item_027'),1)
                    self.assertEqual(count('item_027'),1)
                    self.assertEqual(self.sql(f"SELECT COUNT(*) FROM p1_reward_deliveries WHERE player_id={owner.playerId} AND source_key='activity:{key}' AND claimed_at>0;").strip(),'1')
                    titles=http('CSTitleList',{},owner,'SCTitleList')
                    wanted=['title_022','title_023','title_024','title_025','title_026']
                    self.assertEqual([v.titleId for v in titles.titleList if v.titleId in wanted and v.state==1],wanted)
                    opened=http('CSItemUse',{'costItem':{'id':'item_027','num':1}},owner,'SCItemUse')
                    self.assertEqual(count('item_027'),0);self.assertTrue(opened.get.equips or opened.get.simps)
                    http('CSPvpFindMatch',{},defender,'SCPvpFindMatch')
                    lost=http('CSPvpFightResult',{'playerId':owner.playerId,'serverId':1},defender,'SCPvpFightResult')
                    self.assertEqual(lost.winPlayerId,owner.playerId);self.assertEqual([facts(v)for v in[owner,defender]],[1,0])
                    after=self.profile(owner)
                    self.stop();self.start();owner=self.login(names[1])
                    self.assertEqual(self.call('CSPvpFightResult',args,owner,fight_stamp,'SCPvpFightResult'),result)
                    self.assertEqual(self.profile(owner),after);self.assertEqual(facts(owner),1)
                    cold=http('CSActivityInfo',{'activityId':key},owner,'SCActivityInfo');self.assertEqual(cold.itemList[0].status,2)
                    again=self.call('CSActivityReward',request,owner,claim_stamp,'SCActivityReward');self.assertEqual(len(again.get.simps),0);self.assertEqual(count('item_027'),0)
                    report={'status':'PASS_ORIGINAL_TYPE9_REAL_ARENA_WIN_P1_CLAIM_BOX_TITLES_COLD','sourceSHA256':source_hashes,'managedHelperSHA256':helper_sha,'finitePrerequisites':'Only original CreatePlayer047 plus isolated level100/20; prepublished local activity Goal1/item027x1. No currency/winner/rank/Best/fact/progress/title/GUID/box injection.','actualOwnerEnergySpent':2,'actualLoserEnergySpent':2,'actualWinFacts':[1,0],'titleIDs':wanted,'actualOriginalBoxResult':str(opened.get),'sameFightWorkers':8,'sameActivityClaimWorkers':8,'singleVisibleType9':key,'coldRestarts':1,'applicationAndFixtureTemporaryAccount':True,'productionModified':False,'phoneUI':False}
                finally:
                    try:self.stop()
                    finally:
                        try:
                            if connection is not None:self.managed_pool.release(connection.database)
                        finally:self.managed_pool.close()
                    report['managedAllocationsRemoved']=True
        for rel,expected in source_hashes.items():self.assertEqual(hashlib.sha256((self.source/rel).read_bytes()).hexdigest(),expected)
        report['seconds']=round(time.monotonic()-began,3)
        (ROOT/'analysis/p4/arena-win-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
