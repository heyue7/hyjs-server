"""原等级/战力前三膜拜、真实金币消费与冷恢复；只使用专属InnoDB库。"""
import concurrent.futures
import json
import os
from pathlib import Path
import secrets
import subprocess
import unittest
from server.codec import BY_NAME
import test_p10_mysql as transport

ROOT=Path(__file__).resolve().parents[1]
SOURCE=Path(os.environ.get('HYJS_P10_WORSHIP_SOURCE_ROOT',str(ROOT))).resolve()
if SOURCE!=ROOT and not SOURCE.name.startswith('hyjs-p10-worship-'):
    raise AssertionError('worship source isolation guard')


@unittest.skipUnless(os.environ.get('HYJS_P10_WORSHIP_MYSQL_TEST')=='1','显式启用原膜拜专属MySQL')
class P10WorshipMySQLTests(transport.P10MySQLTests):
    test_1_innodb_real_relationship_concurrency_private_permissions=None
    test_2_two_accounts_original_http_tcp_offline_and_session_rotation=None
    test_3_native_friend_energy_real_give_claim_flags_and_restart=None
    test_4_original_full_actual_formation_profile_permissions_and_restart=None
    test_5_three_real_tcp_scene_move_line_visibility_session_and_restart=None
    test_6_initial_original_login_queries_work_with_default_closed_policy=None
    test_7_original_world_field_text_visibility_black_retry_history_and_restart=None
    test_8_original_fireworks_real20_bags_shared_throw_claim_race_and_restart=None

    @classmethod
    def setUpClass(cls):
        # 用真实共享源码的固定副本，所有既有初始化/注册必须已经在副本中。
        transport.ROOT=SOURCE
        super().setUpClass()
        if any(getattr(cls,name,False) for name in ['used_pending_patch','used_energy_patch','used_profile_patch','used_scene_patch','used_channel_patch','used_lucky_patch']):
            raise AssertionError('worship suite forbids pending integration patches')

    def test_1_mysql_race_original_daily_three_sources_receipts_and_cold(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP10Worship','-count=1','-timeout=120s'],cwd=SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=150)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p10_worship_%';").strip(),'InnoDB')

    def test_2_original_http_tcp_rank_worship_coin_source_consumption_restart_closed_and_no_double(self):
        self.start()
        uids=['p10-worship-native-'+secrets.token_hex(4) for _ in range(5)]
        logins=[self.login(uid) for uid in uids]
        for index,login in enumerate(logins):self.call('CSPlayerCreate',{'playerName':'真实膜拜'+str(index),'heroId':'hero_047'},login,2,'SCPlayerCreate')
        self.login('p10-worship-uncreated-'+secrets.token_hex(4))
        watcher=logins[0];targets=logins[1:4];outsider=logins[4]
        # 三个真实榜单角色的等级是明确的隔离准备；观察者金币从真实初始0开始。
        for index,login in enumerate(targets):
            self.sql('UPDATE profiles SET player=JSON_SET(player,"$.level",'+str(100-index)+'),game=JSON_SET(game,"$.warriors[0].level",'+str(80-index)+') WHERE id='+str(login.playerId)+';')
        self.sql('UPDATE p10_social_policies SET enabled=0; UPDATE p10_worship_policies SET enabled=0;')
        closed=self.call('CSFamous',{'type':1},watcher,10,'SCFamous')
        self.assertEqual((closed.leftAddGood,len(closed.playerList)),(0,3));self.assertEqual([entry.playerId for entry in closed.playerList],[entry.playerId for entry in targets])
        self.assertEqual(self.profile(watcher)[0].get('coin',0),0)
        for name,fields,stamp in [('CSFamous',{'type':-1},10),('CSFamous',{'type':2},10),('CSGood',{'playerId':0},10),('CSGood',{'playerId':targets[0].playerId},0),('CSGood',{'playerId':targets[0].playerId},10)]:
            op,_=self.stamped(name,fields,watcher,stamp);self.assertEqual(op,10001)
        self.sql('UPDATE p10_worship_policies SET enabled=1;')
        for kind in [0,1]:
            ranked=self.call('CSFamous',{'type':kind},watcher,11,'SCFamous')
            self.assertEqual(ranked.leftAddGood,3);self.assertEqual([entry.playerId for entry in ranked.playerList],[entry.playerId for entry in targets]);self.assertTrue(all(entry.serverId==1 for entry in ranked.playerList))
        # protobuf-net的type0可省略，读取仍必须是原战力榜。
        power=self.call('CSFamous',{},watcher,11,'SCFamous');self.assertEqual([entry.playerId for entry in power.playerList],[entry.playerId for entry in targets])
        before=self.profile(watcher)
        op,_=self.stamped('CSGood',{'playerId':outsider.playerId},watcher,12);self.assertEqual(op,10001);self.assertEqual(self.profile(watcher),before)
        request={'playerId':targets[0].playerId}
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSGood',request,watcher,20,'SCGood'),range(8)))
        self.assertTrue(all(reply==replies[0] for reply in replies));first=replies[0]
        self.assertEqual((first.leftAddGood,list(first.goodList)),(2,[targets[0].playerId]));self.assertEqual([(item.id,item.num) for item in first.get.simps],[('coin',10000)])
        self.assertEqual(self.profile(watcher)[0]['coin'],10000)
        for changed,stamp in [(request,21),({'playerId':targets[1].playerId},20)]:
            op,_=self.stamped('CSGood',changed,watcher,stamp);self.assertEqual(op,10001)
        for index,login in enumerate(targets[1:]):self.call('CSGood',{'playerId':login.playerId},watcher,30+index,'SCGood')
        self.assertEqual(self.profile(watcher)[0]['coin'],30000)
        self.assertTrue(all(self.profile(login)[0].get('coin',0)==0 for login in targets))
        for kind in [0,1]:
            ranked=self.call('CSFamous',{'type':kind},watcher,40,'SCFamous');self.assertEqual((ranked.leftAddGood,len(ranked.goodList)),(0,3))
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p10_worship_actions WHERE player_id='+str(watcher.playerId)+';').strip(),'3')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p1_event_facts WHERE player_id='+str(watcher.playerId)+" AND kind='rank_worship';").strip(),'3')
        # 一件已有装备是有限准备；金币全部来自本轮真实膜拜，随后使用原强化路径消费。
        equipment={'guid':'worship-owned-equip','equipId':'equip_001','level':1,'refineLv':0,'refineExp':0,'star':0,'starExp':0,'lastHeroId':'','coinPrice':1000,'gems':[],'forgeLevel':0}
        self.sql('UPDATE profiles SET game=JSON_SET(game,"$.equipment",CAST('+json.dumps(json.dumps([equipment],separators=(',',':')))+' AS JSON)) WHERE id='+str(watcher.playerId)+';')
        self.call('CSFormationOn',{'id':'equip_001','guid':equipment['guid'],'formatPos':1},watcher,41,'SCFormationOn')
        strengthened=self.call('CSEquipStrengthen',{'guid':equipment['guid'],'times':1,'cost':[{'id':'coin','num':8}]},watcher,42,'SCEquipStrengthen')
        self.assertEqual((strengthened.equip.level,strengthened.cost[0].num),(2,8));self.assertEqual(self.profile(watcher)[0]['coin'],29992)
        # 原UI只检查实际展示与已拜列表，本地资格同样保留榜内自己。
        own=self.call('CSGood',{'playerId':targets[0].playerId},targets[0],50,'SCGood')
        self.assertEqual((own.leftAddGood,list(own.goodList)),(2,[targets[0].playerId]));self.assertEqual(self.profile(targets[0])[0]['coin'],10000)
        self.sql('UPDATE p10_worship_policies SET enabled=0;')
        self.assertEqual(self.call('CSGood',request,watcher,20,'SCGood'),first)
        op,_=self.stamped('CSGood',{'playerId':outsider.playerId},watcher,43);self.assertEqual(op,10001)
        self.stop();self.start();watcher=self.login(uids[0])
        self.assertEqual(self.profile(watcher)[0]['coin'],29992)
        self.assertEqual(self.call('CSGood',request,watcher,20,'SCGood'),first)
        game=self.profile(watcher)[1];self.assertEqual(next(item['level'] for item in game['equipment'] if item['guid']==equipment['guid']),2)
        sock=self.connect(watcher);self.tcp_send(sock,'CSFamous',{'type':1},watcher);op,ranked=self.recv(sock)
        self.assertEqual((op,ranked.leftAddGood,len(ranked.goodList)),(BY_NAME['SCFamous'],0,3))
        self.assertEqual(self.sql('SELECT enabled FROM p10_social_policies;').strip(),'0')


if __name__=='__main__':unittest.main()
