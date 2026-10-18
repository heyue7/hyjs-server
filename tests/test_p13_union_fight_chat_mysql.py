"""公会战原type6/18指挥：专库实际六会双实例TCP交流、并发回执与冷恢复。"""
import concurrent.futures
import os
from pathlib import Path
import subprocess
import unittest
import test_p13_union_fight_field_mysql as field
from server.codec import BY_NAME

@unittest.skipUnless(os.environ.get('HYJS_P13_CHAT_MYSQL_TEST')=='1','显式启用战场交流专属MySQL')
class P13UnionFightChatMySQLTests(field.P13UnionFightFieldMySQLTests):
    test_1_mysql_race_actual25_map_legacy_sha_movement_restart_end=None
    test_2_native_original_frames_private_instances_relogin_end_restart=None

    def test_1_mysql_race_original18_text_scope_eight_receipts_cold(self):
        result=subprocess.run(['rtk','proxy','go','test','-race','./service','-run','^TestP13UnionFightChatActual','-count=1','-timeout=180s'],cwd=field.SOURCE/'hyjs-server',env=self.env,capture_output=True,text=True,timeout=210)
        self.assertEqual(result.returncode,0,(result.stdout+result.stderr).replace(self.test_password,'[redacted]'))
        print(result.stdout.strip())
        self.assertEqual(self.sql("SELECT DISTINCT ENGINE FROM information_schema.TABLES WHERE TABLE_SCHEMA='"+self.database+"' AND TABLE_NAME LIKE 'p13_union_fight_chat%';").strip(),'InnoDB')

    def test_2_native_real6guilds_original18_private_chat_cold(self):
        if os.environ.get('HYJS_P13_CHAT_NATIVE_TEST')!='1':self.skipTest('显式启用战场交流原HTTP/TCP')
        return field.P13UnionFightFieldMySQLTests.test_2_native_original_frames_private_instances_relogin_end_restart(self)

    def check_chat_native(self,a,b,c,friend,sa,sb,sc,friend_sock,uids,selected,first):
        chat={'chatType':6,'content':'上交资源保护工厂'}
        op,_=self.stamped('CSChatTcp',chat,a,2000)
        self.assertEqual(op,10001)
        self.sql('UPDATE p13_union_fight_chat_policies SET text_enabled=1,talk_enabled=1;')
        message=self.call('CSChatTcp',chat,a,2000,'SCChatTcp')
        self.assertEqual((message.chatType,message.play.playerId,message.content),(6,a.playerId,chat['content']))
        self.assertEqual(self.receive(friend_sock,'SCChatTcp'),message)
        self.assertFalse(any(op==BY_NAME['SCChatTcp'] for op,_ in self.drain(sb)+self.drain(sc)))
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
            replies=list(pool.map(lambda _:self.call('CSChatTcp',chat,a,2000,'SCChatTcp'),range(8)))
        self.assertTrue(all(reply==message for reply in replies))
        self.assertFalse(any(op==BY_NAME['SCChatTcp'] for op,_ in self.drain(friend_sock)))
        self.tcp_send(sa,'CSChatTcp',chat,a)
        self.assertEqual(self.receive(sa,'SCChatTcp'),message)
        self.assertFalse(any(op==BY_NAME['SCChatTcp'] for op,_ in self.drain(friend_sock)))
        for body,stamp in [({'chatType':6,'content':'改写'},2000),({'chatType':6,'content':'过早'},2001),({'chatType':6,'content':' '},2002),({'chatType':6,'content':'汉'*41},2003),({'chatType':6,'content':'bad\x00'},2004)]:
            self.assertEqual(self.stamped('CSChatTcp',body,a,stamp)[0],10001)
        self.tcp_send(friend_sock,'CSUFChatCache',{},friend)
        history=self.receive(friend_sock,'SCUFChatCache')
        self.assertEqual(len(history.chatList),1)
        self.assertEqual((history.chatList[0].content,history.chatList[0].play.playerId),(chat['content'],a.playerId))
        self.assertEqual(self.call('CSChatCache',{'chatType':6},friend,2009,'SCChatCache').chatList,history.chatList)
        self.assertEqual(len(self.call('CSUFChatCache',{},b,2010,'SCUFChatCache').chatList),0)
        self.assertEqual(len(self.call('CSUFChatCache',{},c,2010,'SCUFChatCache').chatList),0)
        commands=[]
        rows=(field.SOURCE/'analysis/tables/TableText.tsv').read_text().splitlines()
        texts=dict(line.split('\t',1) for line in rows if '\t' in line)
        for line in (field.SOURCE/'analysis/tables/TableUnionFightTalk.tsv').read_text().splitlines()[5:]:
            if not line.strip():continue
            parts=line.split('\t');commands.append(texts[parts[2]])
        self.assertEqual(len(commands),18)
        self.call('CSUFOnHook',{'onHook':1},a,2100,'SCUFOnHook')
        self.assertEqual(self.stamped('CSUFTalk',{'content':commands[0]},a,2200)[0],10001)
        self.call('CSUFOnHook',{'onHook':0},a,2101,'SCUFOnHook')
        self.drain(sa);self.drain(sb);self.drain(sc);self.drain(friend_sock)
        self.assertEqual(self.stamped('CSUFTalk',{'content':'伪造战果'},a,2201)[0],10001)
        self.assertEqual(self.stamped('CSUFTalk',{'talkType':1,'content':commands[0]},a,2202)[0],10001)
        for i,content in enumerate(commands):
            self.set_clock(first-60000+i*20000)
            reply=self.call('CSUFTalk',{'content':content},friend,2300+i,'SCUFTalk')
            self.assertEqual((reply.talkType,reply.content,reply.play.playerId,reply.time),(0,content,friend.playerId,first-60000+i*20000))
            self.assertEqual(self.receive(sa,'SCUFTalk'),reply)
            self.assertFalse(any(op==BY_NAME['SCUFTalk'] for op,_ in self.drain(sb)+self.drain(sc)))
            if i==0:
                again=self.call('CSUFTalk',{'content':content},friend,2300,'SCUFTalk')
                self.assertEqual(again,reply)
                self.assertFalse(any(op==BY_NAME['SCUFTalk'] for op,_ in self.drain(sa)))
                self.assertEqual(self.stamped('CSUFTalk',{'content':commands[1]},friend,2400)[0],10001)
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_chats WHERE kind=0;').strip(),'1')
        self.assertEqual(self.sql('SELECT COUNT(*) FROM p13_union_fight_chats WHERE kind=1;').strip(),'18')
        self.stop();self.sql('UPDATE p13_union_fight_chat_policies SET text_enabled=0,talk_enabled=0;');self.start()
        a,b,c=[self.login(uids[i]) for i in selected]
        sa,sb,sc=[self.connect(x) for x in [a,b,c]]
        friend=self.login(uids[selected[0]+6]);friend_sock=self.connect(friend)
        self.assertEqual(self.call('CSChatTcp',chat,a,2000,'SCChatTcp'),message)
        self.assertEqual(self.call('CSUFChatCache',{},friend,2500,'SCUFChatCache'),history)
        self.assertEqual(len(self.call('CSUFChatCache',{},c,2500,'SCUFChatCache').chatList),0)
        self.call('CSDismissViceChairman',{'playerId':friend.playerId},a,2600,'SCDismissViceChairman')
        self.assertEqual(self.stamped('CSUFTalk',{'content':commands[-1]},friend,2317)[0],10001)
        self.call('CSQuitUnion',{},friend,2700,'SCQuitUnion')
        self.assertEqual(self.stamped('CSUFChatCache',{},friend,2701)[0],10001)
        self.set_clock(first+300000)
        self.assertEqual(self.stamped('CSChatTcp',chat,a,2000)[0],10001)
        self.assertEqual(self.stamped('CSUFChatCache',{},a,2800)[0],10001)

if __name__=='__main__':unittest.main()
