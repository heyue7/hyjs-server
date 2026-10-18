"""P11真实名册/冻结双方/原HTTP与TCP门禁，独立InnoDB库结束清理。"""
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import unittest
from hyjs_managed_mysql import ManagedMySQL
from server.codec import BY_NAME, message
import test_p10_mysql as transport
ROOT=Path(__file__).resolve().parents[1]

@unittest.skipUnless(os.environ.get('HYJS_P11_MYSQL_TEST')=='1','显式启用P11专属MySQL')
class P11MySQLTests(unittest.TestCase):
    send=transport.P10MySQLTests.send
    login=transport.P10MySQLTests.login
    stamped=transport.P10MySQLTests.stamped
    call=transport.P10MySQLTests.call
    profile=transport.P10MySQLTests.profile
    recv=transport.P10MySQLTests.recv
    tcp_send=transport.P10MySQLTests.tcp_send
    connect=transport.P10MySQLTests.connect
    start=transport.P10MySQLTests.start
    stop=transport.P10MySQLTests.stop
    setUp=transport.P10MySQLTests.setUp
    @classmethod
    def setUpClass(cls):
        cls.managed=ManagedMySQL();cls.addClassCleanup(cls.managed.close)
        cls.connection=cls.managed.allocate('p11_legacy')
        cls.database=cls.connection.database;cls.test_user=cls.connection.user;cls.test_password=cls.connection.password
        cls.temp=tempfile.TemporaryDirectory(prefix='hyjs-p11-isolated-');cls.addClassCleanup(cls.temp.cleanup);cls.binary=str(Path(cls.temp.name)/'hyjs-p11')
        cls.env=dict(os.environ,**cls.connection.env())
        cls.env.update(GOTOOLCHAIN='go1.27.2',GOMEMLIMIT='1GiB')
        cls.env.update({'HYJS_P11_MYSQL_'+key:cls.env['HYJS_MYSQL_'+key] for key in ['HOST','USER','PASSWORD','DATABASE']})
        cls.used_pending_patch='InitializeP11Arena(ctx)' not in (ROOT/'hyjs-server/main.go').read_text()
        if cls.used_pending_patch:raise AssertionError('P11需要先合入共享接点，再验证实际源码')
        r=subprocess.run(['rtk','proxy','go','build','-o',cls.binary,'.'],cwd=ROOT/'hyjs-server',env=cls.env,capture_output=True,text=True,timeout=180)
        if r.returncode:raise AssertionError(r.stdout+r.stderr)
    @classmethod
    def database_sql(cls,text):
        if not cls.database.startswith('hyjs_test_p11_'):raise AssertionError('P11 managed isolation guard')
        return cls.managed.sql(cls.database,text)

    def sql(self,text):return self.database_sql(text)
    def test_1_actual_go_mysql_registration_freeze_restart_and_gate(self):
        args=['rtk','proxy','go','test','./service','-run','^TestP11(Arena|FriendDuelReal|Limit|Plunder|Grab)','-count=1','-timeout=120s']
        if os.environ.get('HYJS_P11_GO_RACE')=='1':args.insert(4,'-race')
        r=subprocess.run(args,cwd=ROOT/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
    def test_2_original_http_tcp_real_opponents_receipts_restart_and_no_fake_win(self):
        self.start();uids=['p11-native-'+secrets.token_hex(4) for _ in range(3)];a,b,c=[self.login(uid) for uid in uids]
        for login,name in [(a,'原竞技甲'),(b,'原竞技乙'),(c,'原竞技丙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        ids=','.join(str(x.playerId) for x in [a,b,c])
        self.sql('UPDATE p11_arena_policies SET enabled=0,duel_enabled=0; UPDATE p10_scene_policies SET enabled=0; UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id IN ('+ids+');')
        before=[self.profile(x) for x in [a,b,c]]
        rank=self.call('CSPvpRank',{},a,11,'SCPvpRank');self.assertEqual(rank.myrank.info.playerId,a.playerId);self.assertGreater(rank.myrank.pvpRank,0)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:queries=list(pool.map(lambda _:self.call('CSPvpFindMatch',{},a,12,'SCPvpFindMatch'),range(8)))
        query=queries[0];self.assertTrue(all(q==query for q in queries));actual=[x.info.playerId for x in query.rankList]
        self.assertIn(b.playerId,actual);self.assertIn(c.playerId,actual);self.assertNotIn(a.playerId,actual);self.assertTrue(all(x.info.fightPower>0 for x in query.rankList))
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_receipts WHERE player_id='+str(a.playerId)+' AND stamp=12;').strip(),'1')
        frozen=self.sql('SELECT CONCAT(id,"|",opponent_id,"|",HEX(attacker),"|",HEX(defender),"|",seed,"|",snapshot_hash) FROM p11_arena_matches WHERE viewer_id='+str(a.playerId)+' ORDER BY id;')
        # 原TCP同样查询真实对手；不创建伪造胜利或资源变化。
        sock=self.connect(a);self.tcp_send(sock,'CSPvpFindMatch',{},a);op,tcpquery=self.recv(sock);self.assertEqual(op,BY_NAME['SCPvpFindMatch']);self.assertEqual([x.info.playerId for x in tcpquery.rankList],actual)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_members WHERE best_won_rank>0;').strip(),'0')
        for request,args in [('CSPvpFightResult',{'playerId':b.playerId,'serverId':1}),('CSPvpFight',{'playerId':b.playerId,'serverId':1}),('CSFriendFight',{'playerId':b.playerId,'serverId':1}),('CSPvpFightEnd',{'fightId':'fake-client-win','winPlayerId':a.playerId}),('CSPvpFightResultGuide',{'playerId':b.playerId,'serverId':1})]:
            op,r=self.stamped(request,args,a,20);self.assertEqual(op,10001,str(r))
        # SQL仅打开开关也不能绕过编译期NativeCompared事实；客户端winPlayerId不进入发奖。
        self.sql('UPDATE p11_arena_policies SET enabled=1,duel_enabled=1; UPDATE p6_combat_rules SET authoritative_enabled=1;')
        op,r=self.stamped('CSPvpFightResult',{'playerId':b.playerId,'serverId':1},a,21);self.assertEqual(op,10001,str(r))
        for args in [{'playerId':a.playerId,'serverId':1},{'playerId':b.playerId,'serverId':2},{'playerId':0,'serverId':1}]:
            op,r=self.stamped('CSPvpFightResult',args,a,22);self.assertEqual(op,10001,str(r))
        self.assertEqual([self.profile(x) for x in [a,b,c]],before)
        # 有限更改专库对手英雄等级，旧匹配与回执保持原快照，新查询才刷新实际名册。
        self.sql('UPDATE profiles SET game=JSON_SET(game,"$.warriors[0].level",2) WHERE id='+str(b.playerId)+';')
        self.assertEqual(self.call('CSPvpFindMatch',{},a,12,'SCPvpFindMatch'),query)
        self.call('CSPvpFindMatch',{},a,13,'SCPvpFindMatch')
        original_ids=[line.split('|')[0] for line in frozen.strip().splitlines()]
        old_now=self.sql('SELECT CONCAT(id,"|",opponent_id,"|",HEX(attacker),"|",HEX(defender),"|",seed,"|",snapshot_hash) FROM p11_arena_matches WHERE id IN ('+','.join("'"+v+"'" for v in original_ids)+') ORDER BY id;');self.assertEqual(old_now,frozen)
        after=[self.profile(x) for x in [a,b,c]];sock.close();self.stop();self.start();a=self.login(uids[0]);self.assertEqual(self.call('CSPvpFindMatch',{},a,12,'SCPvpFindMatch'),query)
        self.assertEqual([self.profile(x) for x in [a,b,c]],after)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_receipts WHERE player_id='+str(a.playerId)+' AND operation<>"match-query";').strip(),'0')
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p11_%';").strip(),'InnoDB')
        print('P11 actual shared source, pending patch used:',self.used_pending_patch)

    def test_3_original_friend_relationship_prepares_type11_and_black_revokes_snapshot(self):
        self.start();a,b=[self.login('p11-duel-native-'+secrets.token_hex(4)) for _ in range(2)]
        for login,name in [(a,'原切磋甲'),(b,'原切磋乙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p10_social_policies SET enabled=1;')
        self.call('CSApplyFriend',{'friendId':b.playerId,'word':'原好友切磋关系'},a,100,'SCApplyFriend');self.call('CSAgreeApply',{'friendId':a.playerId},b,101,'SCAgreeApply')
        before=[self.profile(x) for x in [a,b]]
        def probe(denied):
            env=dict(self.env,HYJS_P11_NATIVE_VIEWER=str(a.playerId),HYJS_P11_NATIVE_TARGET=str(b.playerId),HYJS_P11_NATIVE_DUEL_DENIED='1' if denied else '0')
            r=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP11NativeDuelSnapshot$','-count=1'],cwd=ROOT/'hyjs-server',env=env,capture_output=True,text=True,timeout=120)
            self.assertEqual(r.returncode,0,(r.stdout+r.stderr).replace(self.test_password,'[redacted]'));print(r.stdout.strip())
        probe(False)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_duels WHERE viewer_id='+str(a.playerId)+';').strip(),'1')
        op,r=self.stamped('CSFriendFight',{'playerId':b.playerId,'serverId':1},a,102);self.assertEqual(op,10001,str(r));self.assertEqual([self.profile(x) for x in [a,b]],before)
        self.call('CSSendToBlackList',{'friendId':a.playerId},b,103,'SCSendToBlackList');probe(True)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_friend_duels WHERE viewer_id='+str(a.playerId)+';').strip(),'1')
        self.assertEqual([self.profile(x) for x in [a,b]],before)

    def test_4_original_limit_arena_period_score_purchase_retry_and_restart(self):
        self.start();uids=['p11-limit-native-'+secrets.token_hex(4) for _ in range(2)];a,b=[self.login(uid) for uid in uids]
        for login,name in [(a,'原限时甲'),(b,'原限时乙')]:self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p11_limit_policies SET enabled=0,reward_enabled=0; UPDATE p10_scene_policies SET enabled=0; UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id IN ('+str(a.playerId)+','+str(b.playerId)+');')
        before=self.profile(a)
        op,r=self.stamped('CSLimitPvpFindMatch',{},a,100);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(a),before)
        # 显式隔离日历只用于测试；正式活动日期未知，默认关闭。
        self.sql('UPDATE p11_limit_policies SET enabled=1,period_id="native-isolated-a",starts_at=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS SIGNED)-3600000,ends_at=CAST(UNIX_TIMESTAMP(NOW(3))*1000 AS SIGNED)+172800000;')
        self.call('CSLimitPvpFindMatch',{},b,101,'SCLimitPvpFindMatch')
        first=self.call('CSLimitPvpFindMatch',{},a,101,'SCLimitPvpFindMatch');self.assertEqual((first.myrank.info.playerId,first.myrank.pvpIntegral,first.firstRewardScore,first.fightTimes,first.allTimes),(a.playerId,10,10,0,10));self.assertEqual([x.info.playerId for x in first.rankList],[b.playerId]);self.assertEqual(first,self.call('CSLimitPvpFindMatch',{},a,101,'SCLimitPvpFindMatch'))
        second=self.call('CSLimitPvpFindMatch',{},a,102,'SCLimitPvpFindMatch');self.assertEqual((second.myrank.pvpIntegral,second.firstRewardScore),(10,0));self.assertEqual(self.profile(a),before)
        # 限定专库P1领奖中心来源，以原HTTP领取300钻作为购买输入。
        self.sql('INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES('+str(a.playerId)+',"p11-limit-finite-fixture","p11 limit fixture","测试限定","diamond,300",1,0,0);')
        inbox=self.call('CSQueryUserReward',{},a,103,'SCQueryUserReward');reward=next(x for x in inbox.rewardItemInfoList if x.rewardTitle=='p11 limit fixture')
        self.call('CSAddRewardsToPlayer',{'rewardIds':[reward.rewardId]},a,104,'SCAddRewardsToPlayer');self.assertEqual(self.profile(a)[0]['diamond'],300)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSBuyLimitPvpTimes',{'num':2},a,105,'SCBuyLimitPvpTimes'),range(8)))
        paid=replies[0];self.assertTrue(all(x==paid for x in replies));self.assertEqual((paid.fightTimes,paid.allTimes),(0,12));self.assertEqual([(x.id,x.num) for x in paid.cost],[('diamond',60)]);self.assertEqual(self.profile(a)[0]['diamond'],240)
        for args in [{'num':3},{'num':0},{'num':11}]:op,r=self.stamped('CSBuyLimitPvpTimes',args,a,105);self.assertEqual(op,10001,str(r))
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_receipts WHERE player_id='+str(a.playerId)+' AND operation="limit-buy";').strip(),'1')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(a.playerId)+' AND kind="diamond_spent";').strip(),'1')
        info=self.call('CSLimitPvpRewardInfo',{},a,107,'SCLimitPvpRewardInfo');self.assertEqual(len(info.items),11);self.assertTrue(all(x.state==0 for x in info.items))
        unchanged=[self.profile(x) for x in [a,b]]
        for name,args in [('CSLimitPvpFightResult',{'playerId':b.playerId,'serverId':1}),('CSGetLimitPvpRewardInfo',{'itemId':'1'}),('CSRewardLimitSignList',{}),('CSReceiveLimitMonthSign',{})]:op,r=self.stamped(name,args,a,106);self.assertEqual(op,10001,str(r))
        self.assertEqual([self.profile(x) for x in [a,b]],unchanged)
        sock=self.connect(a);self.tcp_send(sock,'CSLimitPvpFindMatch',{},a);op,tcp=self.recv(sock);self.assertEqual(op,BY_NAME['SCLimitPvpFindMatch']);self.assertEqual((tcp.myrank.pvpIntegral,tcp.firstRewardScore,tcp.fightTimes,tcp.allTimes),(10,0,0,12));sock.close()
        frozen=self.sql('SELECT CONCAT(id,"|",starts_at,"|",ends_at,"|",rules_hash) FROM p11_limit_periods WHERE id="native-isolated-a";')
        self.stop();self.start();a,b=[self.login(uid) for uid in uids];self.assertEqual(paid,self.call('CSBuyLimitPvpTimes',{'num':2},a,105,'SCBuyLimitPvpTimes'));self.assertEqual(self.sql('SELECT CONCAT(id,"|",starts_at,"|",ends_at,"|",rules_hash) FROM p11_limit_periods WHERE id="native-isolated-a";'),frozen)
        self.sql('UPDATE p11_limit_policies SET enabled=0;');self.assertEqual(paid,self.call('CSBuyLimitPvpTimes',{'num':2},a,105,'SCBuyLimitPvpTimes'));op,r=self.stamped('CSBuyLimitPvpTimes',{'num':1},a,110);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(a)[0]['diamond'],240)
        # 累计积分奖只使用已记录的登录分，原10分首档独立运营开关，胜利/日榜仍未开放。
        self.sql('UPDATE p11_limit_policies SET enabled=1,reward_enabled=1;')
        eligible=self.call('CSLimitPvpRewardInfo',{},a,120,'SCLimitPvpRewardInfo');self.assertEqual([x.rewardKey for x in eligible.items],list(range(1,12)));self.assertEqual([x.state for x in eligible.items],[1]+[0]*10)
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:rewards=list(pool.map(lambda _:self.call('CSGetLimitPvpRewardInfo',{'itemId':'1'},a,121,'SCGetLimitPvpRewardInfo'),range(8)))
        awarded=rewards[0];self.assertTrue(all(r==awarded for r in rewards));self.assertEqual([(x.id,x.num) for x in awarded.get.simps],[('frag_hero_082',10)])
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_limit_reward_claims WHERE player_id='+str(a.playerId)+';').strip(),'1')
        done=self.call('CSLimitPvpRewardInfo',{},a,122,'SCLimitPvpRewardInfo');self.assertEqual(done.items[0].state,2)
        saved=self.profile(a);empty=self.call('CSGetLimitPvpRewardInfo',{'itemId':'1'},a,123,'SCGetLimitPvpRewardInfo');self.assertEqual(len(empty.get.simps),0);self.assertEqual(self.profile(a),saved)
        for fields in [{'itemId':'2'},{'itemId':'01'},{'itemId':'999'},{'itemId':'hero_082'}]:op,r=self.stamped('CSGetLimitPvpRewardInfo',fields,a,124);self.assertEqual(op,10001,str(r))
        self.stop();self.start();a=self.login(uids[0]);self.sql('UPDATE p11_limit_policies SET enabled=0,reward_enabled=0;');self.assertEqual(awarded,self.call('CSGetLimitPvpRewardInfo',{'itemId':'1'},a,121,'SCGetLimitPvpRewardInfo'));self.assertEqual(self.profile(a),saved)
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p11_limit_%';").strip(),'InnoDB')
        print('P11 limit original HTTP/TCP, real MySQL, frozen cycle and paid receipt restart verified; default activity closed.')

    def test_5_original_plunder_fragments_protection_receipts_and_combine(self):
        self.start();uid='p11-plunder-native-'+secrets.token_hex(4);a=self.login(uid);self.call('CSPlayerCreate',{'playerName':'原免战甲','heroId':'hero_047'},a,1,'SCPlayerCreate')
        self.sql('UPDATE p11_plunder_policies SET avoid_enabled=0; UPDATE p10_scene_policies SET enabled=0; UPDATE profiles SET player=JSON_SET(player,"$.level",20) WHERE id='+str(a.playerId)+';')
        before=self.profile(a);empty=self.call('CSRefreshTreasFrag',{},a,101,'SCRefreshTreasFrag');self.assertEqual((len(empty.frags),empty.energy,empty.maxEnergy,empty.avoidWarTime),(0,100,30,0));self.assertEqual(self.profile(a),before)
        op,r=self.stamped('CSUseAvoidPlunder',{'usedItem':'item_015'},a,102);self.assertEqual(op,10001,str(r));self.assertEqual(self.profile(a),before)
        # 原P1领奖中心有限隔离石/片源，后续消费与合成全走原协议。
        self.sql('INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES('+str(a.playerId)+',"p11-plunder-finite-fixture","p11 plunder fixture","finite source","item_015,2;item_014,1;frag_trea_001_1,1;frag_trea_001_2,1;frag_trea_001_3,1",1,0,0);')
        inbox=self.call('CSQueryUserReward',{},a,103,'SCQueryUserReward');reward=next(x for x in inbox.rewardItemInfoList if x.rewardTitle=='p11 plunder fixture');self.call('CSAddRewardsToPlayer',{'rewardIds':[reward.rewardId]},a,104,'SCAddRewardsToPlayer')
        query=self.call('CSRefreshTreasFrag',{},a,105,'SCRefreshTreasFrag');self.assertEqual([(x.id,x.num) for x in query.frags],[('frag_trea_001_1',1),('frag_trea_001_2',1),('frag_trea_001_3',1)])
        self.sql('UPDATE p11_plunder_policies SET avoid_enabled=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:uses=list(pool.map(lambda _:self.call('CSUseAvoidPlunder',{'usedItem':'item_015'},a,106,'SCUseAvoidPlunder'),range(8)))
        self.assertTrue(all((x.cost.id,x.cost.num)==('item_015',1) and 3599000<=x.avoidSurplusTime<=3600000 for x in uses))
        stacks={x['id']:x['num'] for x in self.profile(a)[1]['items']};self.assertEqual(stacks['item_015'],1)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_arena_receipts WHERE player_id='+str(a.playerId)+' AND operation="plunder-avoid";').strip(),'1')
        first_until=int(self.sql('SELECT avoid_until FROM p11_plunder_states WHERE player_id='+str(a.playerId)+';').strip())
        op,r=self.stamped('CSUseAvoidPlunder',{'usedItem':'item_014'},a,106);self.assertEqual(op,10001,str(r))
        extended=self.call('CSUseAvoidPlunder',{'usedItem':'item_014'},a,107,'SCUseAvoidPlunder');self.assertTrue(32390000<=extended.avoidSurplusTime<=32400000)
        self.assertEqual(int(self.sql('SELECT avoid_until FROM p11_plunder_states WHERE player_id='+str(a.playerId)+';').strip())-first_until,28800000)
        combined=self.call('CSCombine',{'targetId':'trea_001'},a,108,'SCCombine');self.assertEqual(len(combined.get.treas),1);self.assertEqual(combined.get.treas[0].treasureId,'trea_001');self.assertEqual(len(self.call('CSRefreshTreasFrag',{},a,109,'SCRefreshTreasFrag').frags),0)
        frozen=self.sql('SELECT CONCAT(avoid_until,"|",last_use_at,"|",last_item) FROM p11_plunder_states WHERE player_id='+str(a.playerId)+';');saved=self.profile(a)
        self.stop();self.start();a=self.login(uid);self.assertEqual(self.sql('SELECT CONCAT(avoid_until,"|",last_use_at,"|",last_item) FROM p11_plunder_states WHERE player_id='+str(a.playerId)+';'),frozen)
        self.sql('UPDATE p11_plunder_policies SET avoid_enabled=0,item_hours_json="[]",item_hours_hash="closed_new_policy";')
        repeated=self.call('CSUseAvoidPlunder',{'usedItem':'item_015'},a,106,'SCUseAvoidPlunder');self.assertTrue(32390000<=repeated.avoidSurplusTime<=32400000);self.assertEqual(self.profile(a),saved)
        for name,fields in [('CSTreasureBattleList',{'fragTreasId':'frag_trea_001_1'}),('CSFightTreasureStartResult',{'fightPlayerId':a.playerId+1,'targetFlagId':'frag_trea_001_1'}),('CSFightTreasureEnd',{'fightPlayerId':a.playerId+1,'fragId':'frag_trea_001_1','isWin':1}),('CSSweepTreasureFight',{'fragId':'frag_trea_001_1','sweepNumber':5}),('CSUseAvoidPlunder',{'usedItem':'item_001'})]:op,r=self.stamped(name,fields,a,110);self.assertEqual(op,10001,str(r))
        self.assertEqual(self.profile(a),saved);self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(a.playerId)+' AND kind="treasure_avoid_item_used";').strip(),'2')
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p11_plunder_%';").strip(),'InnoDB')
        print('P11 original plunder fragment query/protection 1+8h, original combine, MySQL repeat/restart verified; authority/probability remain closed.')

    def test_6_original_grab_empty_real_players_purchase_giveup_and_restart(self):
        self.start();uids=['p11-grab-native-'+secrets.token_hex(4) for _ in range(3)];players=[self.login(uid) for uid in uids]
        for login,name in zip(players,['原空位甲','原空位乙','原空位丙']):self.call('CSPlayerCreate',{'playerName':name,'heroId':'hero_047'},login,1,'SCPlayerCreate')
        self.sql('UPDATE p11_grab_policies SET enabled=0,time_reward_enabled=0; UPDATE p10_scene_policies SET enabled=0; UPDATE profiles SET player=JSON_SET(player,"$.level",30) WHERE id IN ('+','.join(str(x.playerId) for x in players)+');')
        empty=self.call('CSGetGrabData',{},players[0],101,'SCGetGrabData');self.assertEqual([x.position for x in empty.grabData],list(range(1,19)));self.assertTrue(all(x.playerId==0 for x in empty.grabData));self.assertEqual((empty.grabTimes,empty.lastCount),(3,3))
        before=[self.profile(x) for x in players];op,r=self.stamped('CSGrab',{'index':1},players[0],102);self.assertEqual(op,10001,str(r));self.assertEqual([self.profile(x) for x in players],before)
        self.sql('UPDATE p11_grab_policies SET enabled=1;')
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:results=list(pool.map(lambda login:(login,self.stamped('CSGrab',{'index':1},login,103)),players))
        winners=[(login,r) for login,(op,r) in results if op==BY_NAME['SCGrab']];self.assertEqual(len(winners),1);winner,occupied=winners[0];loser=next(x for x in players if x.playerId!=winner.playerId)
        self.assertEqual((occupied.grabData.playerId,occupied.grabData.position,occupied.currEnergy,occupied.grabTimes),(winner.playerId,1,90,2));self.assertFalse(occupied.HasField('attPlayer'));self.assertEqual(occupied.winPlayerId,0)
        for login in players:self.assertEqual(self.profile(login)[0]['energy'],90 if login.playerId==winner.playerId else 100)
        op,r=self.stamped('CSGrab',{'index':7},winner,104);self.assertEqual(op,10001,str(r));op,r=self.stamped('CSGiveUpGrab',{'index':1},loser,104);self.assertEqual(op,10001,str(r))
        # 获胜者通过原P1有限钻石来源支付已占空位之后的买次，保留原消费事实。
        self.sql('INSERT INTO p1_reward_deliveries(player_id,source_key,title,content,rewards,created_at,expires_at,claimed_at) VALUES('+str(winner.playerId)+',"p11-grab-finite-fixture","p11 grab fixture","finite source","diamond,300",1,0,0);')
        inbox=self.call('CSQueryUserReward',{},winner,105,'SCQueryUserReward');reward=next(x for x in inbox.rewardItemInfoList if x.rewardTitle=='p11 grab fixture');self.call('CSAddRewardsToPlayer',{'rewardIds':[reward.rewardId]},winner,106,'SCAddRewardsToPlayer')
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:purchases=list(pool.map(lambda _:self.call('CSBuyGrab',{'count':1},winner,107,'SCBuyGrab'),range(8)))
        purchase=purchases[0];self.assertTrue(all(r==purchase for r in purchases));self.assertEqual((purchase.currCount,purchase.lastCount),(3,2));self.assertEqual([(x.id,x.num) for x in purchase.cost],[('diamond',100)]);self.assertEqual(self.profile(winner)[0]['diamond'],200)
        released=self.call('CSGiveUpGrab',{'index':1},winner,108,'SCGiveUpGrab');self.assertEqual(released.grabData.playerId,0)
        taken=self.call('CSGrab',{'index':1},loser,109,'SCGrab');self.assertEqual(taken.grabData.playerId,loser.playerId);self.assertFalse(taken.HasField('attPlayer'))
        self.assertEqual(released,self.call('CSGiveUpGrab',{'index':1},winner,108,'SCGiveUpGrab'));self.assertEqual(self.sql('SELECT owner_id FROM p11_grab_slots WHERE position=1;').strip(),str(loser.playerId))
        second=self.call('CSGrab',{'index':7},winner,110,'SCGrab');self.assertEqual((second.grabData.position,second.grabData.playerId,second.currEnergy),(7,winner.playerId,80))
        op,r=self.stamped('CSGrab',{'index':13},winner,110);self.assertEqual(op,10001,str(r));self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_sessions WHERE player_id='+str(winner.playerId)+';').strip(),'2')
        info=self.call('CSGetTimeRewardInfo',{},winner,111,'SCGetTimeRewardInfo');self.assertGreaterEqual(info.myGrabMinutes,0);self.assertEqual(list(info.rewardTime),[])
        for name,fields in [('CSGetTimeReward',{'time':1}),('CSGetRandReward',{}),('CSGrab',{'index':0}),('CSBuyGrab',{'count':4}),('CSGiveUpGrab',{'index':19})]:op,r=self.stamped(name,fields,winner,112);self.assertEqual(op,10001,str(r))
        frozen=self.sql('SELECT CONCAT(position,"|",owner_id,"|",session_id,"|",started_at,"|",expires_at) FROM p11_grab_slots WHERE owner_id IS NOT NULL ORDER BY position;');saved={x.playerId:self.profile(x) for x in players};winner_id=winner.playerId;loser_id=loser.playerId
        self.stop();self.start();players=[self.login(uid) for uid in uids];winner=next(x for x in players if x.playerId==winner_id);loser=next(x for x in players if x.playerId==loser_id)
        self.assertEqual(self.sql('SELECT CONCAT(position,"|",owner_id,"|",session_id,"|",started_at,"|",expires_at) FROM p11_grab_slots WHERE owner_id IS NOT NULL ORDER BY position;'),frozen)
        self.sql('UPDATE p11_grab_policies SET enabled=0;');self.assertEqual(purchase,self.call('CSBuyGrab',{'count':1},winner,107,'SCBuyGrab'));self.assertEqual({x.playerId:self.profile(x) for x in players},saved)
        self.call('CSGiveUpGrab',{'index':7},winner,113,'SCGiveUpGrab');self.assertEqual(self.sql('SELECT COUNT(*) FROM p11_grab_slots WHERE owner_id='+str(winner.playerId)+';').strip(),'0')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(winner.playerId)+' AND kind="grab_empty_occupied";').strip(),'2');self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(winner.playerId)+' AND kind="spend:energy";').strip(),'2')
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p11_grab_%';").strip(),'InnoDB')
        print('P11 original Grab no-attacker success branch, real-player SQL unique slots/buy/giveup/restart verified; occupied challenge/random reward remain gated.')
