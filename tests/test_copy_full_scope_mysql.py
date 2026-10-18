"""All original chapter contracts and natural specialty book consumption on disposable InnoDB.
Client reported battle wins are explicit protocol fixtures here; original native battle
winner/HP/operation verification is kept as a separate gate and is not claimed by this test.
"""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import time
import unittest
from server.codec import BY_NAME
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport


@unittest.skipUnless(os.environ.get('HYJS_COPY_FULL_MYSQL_TEST')=='1','Explicit isolated full original mainline protocol matrix')
class OriginalCopyFullMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    stamped=transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_all_original_chapters_quests_rewards_books_specialty_and_cold_receipts(self):
        began=time.monotonic();self.start();login=self.login('all-original-copy-native')
        stamp=1800002000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'原主线完整链','heroId':'hero_043'},'SCPlayerCreate')
        player,game=self.profile(login);player.update(level=100,coin=1000000000,diamond=100000,stamina=100000,warriorBagSize=10000,equipBagSize=1000,noviceGuideSchedule=8)
        # Only level/stamina and a finite original hero fragment prerequisite are
        # prepared; zero books439/440 are ever inserted or directly granted.
        game['items']=[{'id':'frag_hero_001','num':150}];self.write_profile(login,player,game)
        chapters=shared.original_table('CopyChapter');missions=shared.original_table('CopyMission');tasks=shared.original_table('CopyTask');quests=shared.original_table('StoryQuest')
        fights=claims=boxes=0;chapter_results=[];last_finish=None;last_result=None;mission42=[]
        for i,mission in enumerate(missions):
            chapter,mid=mission['ChapterId'],mission['MissionId']
            if i+1<len(missions):
                nxt=missions[i+1];self.assertEqual(self.stamped('CSCopyEnter',{'chapterId':nxt['ChapterId'],'missionId':nxt['MissionId']},login,stamp+1)[0],10001)
            entry=call('CSCopyEnter',{'chapterId':chapter,'missionId':mid},'SCCopyEnter')
            original_tasks=[row for row in tasks if row['MissionId']==mid]
            self.assertEqual([x.taskId for x in entry.jobList],[x['TaskId'] for x in original_tasks])
            for task in original_tasks:
                start=call('CSCopyFightStart',{'chapterId':chapter,'missionId':mid,'taskId':task['TaskId']},'SCCopyFightStart')
                self.assertEqual(call('CSCopyFightStart',{'chapterId':chapter,'missionId':mid,'taskId':task['TaskId']},'SCCopyFightStart'),start)
                if mid=='42':mission42.append({'taskId':task['TaskId'],'seed':start.randSeed,'fightStartSha256':hashlib.sha256(start.SerializeToString()).hexdigest()})
                last_finish={'fightId':start.fightId,'result':1,'battlePlayer':start.battlePlayer}
                stamp+=1
                if task['TaskId']=='168':
                    with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSCopyFightEnd',last_finish,login,stamp,'SCCopyFightEnd'),range(8)))
                    self.assertTrue(all(reply==replies[0] for reply in replies));last_result=replies[0]
                else:last_result=self.call('CSCopyFightEnd',last_finish,login,stamp,'SCCopyFightEnd')
                self.assertEqual(self.call('CSCopyFightEnd',last_finish,login,stamp,'SCCopyFightEnd'),last_result)
                fights+=1
            for quest in quests:
                if quest['missionId']!=mid:continue
                result=call('CSQuestReward',{'questId':quest['questId']},'SCQuestReward');self.assertTrue(result.get.ByteSize())
                self.assertEqual(self.stamped('CSQuestReward',{'questId':quest['questId']},login,stamp+1)[0],10001);claims+=1
            if i+1==len(missions) or missions[i+1]['ChapterId']!=chapter:
                for box in range(3):
                    fields={'chapterId':chapter,'starIndex':box};result=call('CSCopyGetStarAward',fields,'SCCopyGetStarAward');self.assertTrue(result.gain.ByteSize());self.assertEqual(self.stamped('CSCopyGetStarAward',fields,login,stamp+1)[0],10001);boxes+=1
                chapter_results.append({'chapterId':chapter,'actualLastMission':mid,'actualStarboxes':3})
        self.assertEqual((len(chapters),len(missions),fights,claims,boxes),(27,161,638,173,81))
        player,game=self.profile(login);books={x['id']:x['num'] for x in game['items']};self.assertGreaterEqual(books.get('item_439',0),20);self.assertGreaterEqual(books.get('item_440',0),15)
        hero=call('CSCombine',{'targetId':'hero_001'},'SCCombine').get.wars[0]
        player,game=self.profile(login);owned=next(x for x in game['warriors'] if x['guid']==hero.guid);self.assertEqual(owned.get('skillRefine',[]),[]);owned['level']=75
        self.write_profile(login,player,game)
        call('CSFormationOn',{'formatPos':2,'id':hero.id,'guid':hero.guid},'SCFormationOn')
        call('CSCopyEnter',{'chapterId':'27','missionId':'161'},'SCCopyEnter')
        base=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo');before=next(x for x in base.battlePlayer.formation if x.guid==hero.guid)
        player,game=self.profile(login);initial_books={x['id']:x['num'] for x in game['items']};fields={'roleGuid':hero.guid,'type':2};stamp+=1;specialty_stamp=stamp
        with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:replies=list(pool.map(lambda _:self.call('CSUpRoleSkillRefineLevel',fields,login,specialty_stamp,'SCUpRoleSkillRefineLevel'),range(8)))
        self.assertTrue(all(x==replies[0] for x in replies));self.assertEqual([(x.id,x.num) for x in replies[0].cost],[('item_439',20),('item_440',15)])
        after_map=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo');after=next(x for x in after_map.battlePlayer.formation if x.guid==hero.guid);self.assertEqual(after.prop.mdef-before.prop.mdef,18)
        player,game=self.profile(login);final_books={x['id']:x['num'] for x in game['items']};self.assertEqual((initial_books['item_439']-final_books['item_439'],initial_books['item_440']-final_books['item_440']),(20,15))
        self.stop();self.start();login=self.login('all-original-copy-native')
        self.assertEqual(self.call('CSUpRoleSkillRefineLevel',fields,login,specialty_stamp,'SCUpRoleSkillRefineLevel'),replies[0])
        cold=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo');self.assertEqual(cold.battlePlayer,after_map.battlePlayer)
        # Sweep all27 last missions; ordinary drops are included with actual stamina,
        # challenge count and a same-frame random receipt before and after restart.
        call('CSCopyLeave',{},'SCCopyLeave')
        last_sweep=None
        for info in chapter_results:
            request={'chapterId':info['chapterId'],'missionId':info['actualLastMission']};stamp+=1
            result=self.call('CSCopySweep',request,login,stamp,'SCCopySweep');self.assertTrue(result.gain.wars)
            self.assertEqual(self.call('CSCopySweep',request,login,stamp,'SCCopySweep'),result);last_sweep=(request,stamp,result)
        self.stop();self.start();login=self.login('all-original-copy-native')
        self.assertEqual(self.call('CSCopySweep',last_sweep[0],login,last_sweep[1],'SCCopySweep'),last_sweep[2])
        report={'date':'2026-10-14','actualChapters':27,'actualMissions':161,'actualEncounters':fights,'actualQuests':claims,'actualStarboxes':boxes,'actualChapterSweeps':27,'originalFramesAndInnoDB':'passed','eightConcurrentTask168Settlement':'one persisted random gain; eight identical original responses','eightConcurrentNaturalSpecialty':'20/15 consumed once, original mdef+18','coldRandomSweepAndSpecialtyReceipts':'identical','booksDirectlyPrepared':False,'mission42Starts':mission42,'chapterPaths':chapter_results,'seconds':round(time.monotonic()-began,3),'sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'productionModified':False,'battleBoundary':'all winners in this complete content protocol regression are explicit isolated client-reported fixtures; original native automatic AI winner/HP/operation source chain remains independent','probabilityBoundary':'declared relative-weights-v1 local compatibility policy; original private server algorithm and SpecialOdds triples remain unknown','preparation':'finite level100/stamina/coin/diamond, original150 fragments; only specialty target75 prerequisite after actual combine, no books inserted'}
        (shared.ROOT/'analysis/p5/copy-full-scope-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')

if __name__=='__main__':unittest.main()
