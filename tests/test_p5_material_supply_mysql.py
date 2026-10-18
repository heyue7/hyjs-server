"""Declared four-book economy on original HTTP/InnoDB; no gold/book reward injection."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import time
import unittest

import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as transport


@unittest.skipUnless(os.environ.get('HYJS_P5_SUPPLY_MYSQL_TEST')=='1','Explicit isolated book-supply economy')
class MaterialSupplyMySQLTests(unittest.TestCase):
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

    def test_source_gold_four_book_purchase_and_actual_strength_consumption(self):
        began=time.monotonic();self.start();login=self.login('natural-book-supply')
        stamp=1800012000000
        def call(name,fields,response):
            nonlocal stamp
            stamp+=1;return self.call(name,fields,login,stamp,response)
        call('CSPlayerCreate',{'playerName':'自然专精书链','heroId':'hero_043'},'SCPlayerCreate')
        p,g=self.profile(login);p.update(level=100,coin=0,noviceGuideSchedule=8)
        missions=shared.original_table('CopyMission')
        # Finite historical chapter/hero/other-material prerequisites are explicit.
        # Source coin and all four exchanged books begin at zero and are never inserted.
        g['copy']['passedCopyInfo']=[{'chapterId':x['ChapterId'],'missionId':x['MissionId'],'star':5,'challengeTimes':0} for x in missions if int(x['ChapterId'])>=20]
        g['copy']['passedChapterInfo']=[]
        self.write_profile(login,p,g)
        source_gold=0
        for chapter in range(20,28):
            for box in range(3):
                result=call('CSCopyGetStarAward',{'chapterId':str(chapter),'starIndex':box},'SCCopyGetStarAward')
                source_gold+=next(x.num for x in result.gain.simps if x.id=='coin')
        self.assertEqual(source_gold,8380000);self.assertEqual(self.profile(login)[0]['coin'],source_gold)
        month=time.strftime('%Y%m',time.gmtime(time.time()+8*3600));key='limitExchange_p4_specialty_'+month
        catalogue=call('CSActivityShopInfo',{'activityId':key},'SCActivityShopInfo')
        self.assertEqual(len(catalogue.shopItemList),4)
        quantities=[9,15,8,10];ids=['item_441','item_445','item_449','item_453'];expected_cost=[50000,100000,200000,400000]
        results=[];cost_total=0
        for index,item in enumerate(catalogue.shopItemList):
            self.assertEqual((item.getResource[0].id,item.getResource[0].num),(ids[index],10))
            self.assertEqual(item.name,['新手技能专精书','熟练技能专精书','专家技能专精书','大师技能专精书'][index])
            self.assertEqual((item.costResource[0].id,item.costResource[0].num),('coin',expected_cost[index]))
            fields={'activityId':key,'index':item.index,'num':quantities[index],'costResources':item.costResource,'getResources':item.getResource}
            bad=dict(fields,costResources=[{'id':'coin','num':1}]);before=self.profile(login)
            self.assertEqual(self.stamped('CSBuyActivityShop',bad,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
            stamp+=1;buy_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies=list(pool.map(lambda _:self.call('CSBuyActivityShop',fields,login,buy_stamp,'SCGetActivityShopReward'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));cost_total+=expected_cost[index]*quantities[index]
            p,g=self.profile(login);self.assertEqual(next(x['num'] for x in g['items'] if x['id']==ids[index]),10*quantities[index])
            if index==3:
                denied=dict(fields,num=1);before=self.profile(login);self.assertEqual(self.stamped('CSBuyActivityShop',denied,login,stamp+1)[0],10001);self.assertEqual(self.profile(login),before)
            saved=self.profile(login);self.stop();self.start();login=self.login('natural-book-supply')
            self.assertEqual(self.call('CSBuyActivityShop',fields,login,buy_stamp,'SCGetActivityShopReward'),replies[0]);self.assertEqual(self.profile(login),saved)
            results.append({'id':ids[index],'bundles':quantities[index],'goldCost':expected_cost[index]*quantities[index],'eightConcurrentAndColdReceipt':True})
        self.assertEqual(cost_total,7550000);self.assertEqual(self.profile(login)[0]['coin'],source_gold-cost_total)
        target={'id':'hero_001','guid':'book-supply-finite-partner','level':100,'exp':0,'talent':0,'training':{},'tempTraining':{},'breached':{'level':1},'awake':{}}
        phases=[(10,1,{'item_442':10},90,1500),(20,2,{'item_444':40,'item_446':10},150,3000),(40,4,{'item_447':70,'item_448':50,'item_450':10},80,5000),(70,7,{'item_451':85,'item_452':60,'item_454':10},100,6000)]
        for index,(level,strength,other,amount,hp_delta) in enumerate(phases):
            p,g=self.profile(login);target['guid']='book-supply-finite-partner-'+str(index);target['skillRefine']=[{'roleGuid':target['guid'],'type':1,'level':level,'strengthLevel':strength}]
            g['warriors']=[x for x in g['warriors'] if x['guid']!=target['guid']]+[dict(target)]
            for resource,num in other.items():
                g['items']=[x for x in g['items'] if x['id']!=resource]+[{'id':resource,'num':num}]
            g['run']={'chapter':'27','mission':'161','passed':[]};self.write_profile(login,p,g)
            call('CSFormationOn',{'formatPos':2,'id':target['id'],'guid':target['guid']},'SCFormationOn')
            before_map=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo')
            before=next(x.prop.hp for x in before_map.battlePlayer.formation if x.guid==target['guid'])
            fields={'roleGuid':target['guid'],'type':1};stamp+=1;strength_stamp=stamp
            with concurrent.futures.ThreadPoolExecutor(max_workers=8) as pool:
                replies=list(pool.map(lambda _:self.call('CSStrengthRoleSkillRefineLevel',fields,login,strength_stamp,'SCStrengthRoleSkillRefineLevel'),range(8)))
            self.assertTrue(all(x==replies[0] for x in replies));self.assertEqual(next(x.num for x in replies[0].cost if x.id==ids[index]),amount)
            after_map=call('CSCopyRunMapInfo',{'chapterId':'27','missionId':'161'},'SCCopyRunMapInfo')
            after=next(x.prop.hp for x in after_map.battlePlayer.formation if x.guid==target['guid'])
            self.assertEqual(after-before,hp_delta)
            p,g=self.profile(login);self.assertEqual(next((x['num'] for x in g['items'] if x['id']==ids[index]),0),0)
            saved=self.profile(login);self.stop();self.start();login=self.login('natural-book-supply')
            self.assertEqual(self.call('CSStrengthRoleSkillRefineLevel',fields,login,strength_stamp,'SCStrengthRoleSkillRefineLevel'),replies[0]);self.assertEqual(self.profile(login),saved)
            results[index].update(actualBooksConsumed=amount,actualBattleHPDelta=hp_delta,consumptionEightConcurrentAndColdReceipt=True)
        report={'date':'2026-10-14','policy':'specialty-mainline-coin-exchange-v1','sourceGold':source_gold,'actualGoldSpent':cost_total,'booksDirectlyPrepared':False,'goldDirectlyPrepared':False,'actualSource':'24 original chapter20..27 starboxes; finite prior completion/star eligibility fixture','results':results,'seconds':round(time.monotonic()-began,3),'originalFramesAndInnoDB':'passed','preparation':'finite level100, chapter20..27 historical stars, hero001 and original lower specialty prerequisite, required other materials; four source books and coin never inserted','originalRuleBoundary':'four book production source absent from all164 recovered tables; price/catalogue/limit/gate are explicit configurable local compatibility, original private server supply unknown','binarySHA256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'ownedSourceSHA256':{k:v for k,v in self.source_shas.items() if k in ['service/p5MaterialSupplyService.go','service/p4CommerceService.go','service/p4ActivityService.go','constUtil/p5MaterialSupply.go']},'cleanup':'dedicated schema/user/grants and temporary source/binary removed by class cleanup','productionModified':False}
        (shared.ROOT/'analysis/p5/material-supply-mysql-validation.json').write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
