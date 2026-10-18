"""Every original equipment at260/80 and all legal5-stars, exact reset and restart."""
import concurrent.futures
import hashlib
import json
import math
import os
from pathlib import Path
import subprocess
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_EQUIPMENT_MYSQL_TEST') == '1', 'Explicit isolated highest equipment test')
class EquipmentHighestMySQLTests(unittest.TestCase):
    setUpClass = classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql = classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send = shared.TalentScopeMySQLTests.send
    login = shared.TalentScopeMySQLTests.login
    stamped = stressed_transport.GameCreationMySQLTests.stamped
    call = shared.TalentScopeMySQLTests.call
    profile = shared.TalentScopeMySQLTests.profile
    start = shared.TalentScopeMySQLTests.start
    stop = shared.TalentScopeMySQLTests.stop
    setUp = shared.TalentScopeMySQLTests.setUp
    sql = shared.TalentScopeMySQLTests.sql
    write_profile = shared.TalentScopeMySQLTests.write_profile

    def test_all36_equipment260_refine80_and_legal_stars5(self):
        domain_result='independent regression required'
        if os.environ.get('HYJS_P5_EQUIPMENT_NATIVE_ONLY') != '1':
            domain=subprocess.run(['rtk','proxy','go','test','./service','-run','^TestP5OriginalEquipmentHighestFullScope$','-count=1','-timeout','600s'],cwd=self.frozen_source,env=self.env,text=True,capture_output=True,timeout=630)
            self.assertEqual(domain.returncode,0,'Dedicated InnoDB highest equipment domain failed')
            self.assertNotIn('no tests to run',domain.stdout+domain.stderr)
            domain_result='passed'
        self.start()
        configs=shared.original_table('Equip')
        self.assertEqual(len(configs),36)
        exp_rows=shared.original_table('EquipExp')
        refine_rows=shared.original_table('EquipRefineExp')
        stars=shared.original_table('EquipStar')
        forge_rows=shared.original_table('EquipForgeOdds')
        resources=('item_008','item_459','item_460','item_461','item_462','item_463','item_464')
        prepared=[];results=[];restart=[]
        for index,config in enumerate(configs):
            uid='equipment-highest-native-'+config['id']
            login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'原装备最高成长','heroId':'hero_043'},login,1800001000000+index,'SCPlayerCreate')
            player,game=self.profile(login)
            player.update(level=100,coin=1000000000,diamond=1000,equipBagSize=1000,noviceGuideSchedule=8)
            identity=config['id']+'-highest-native'
            # Zero-stage fixtures stand only for finite original instance supply.
            # Dedicated domain above acquires all36 through real GrantFixedReward.
            game['equipment']=[{'equipId':config['id'],'guid':identity,'level':1,'refineLv':0,'refineExp':0,'star':0,'forgeLevel':0,'gems':[]}]
            game['items']=[{'id':r,'num':100000} for r in resources]
            if int(config['quality'])>=20:game['items'].append({'id':'frag_'+config['id'],'num':100000})
            game['formation']=[{'formatPos':1,'battlePos':1,'things':[{'pos':0,'id':'hero_043','guid':player['heroWarId']}]}]
            self.write_profile(login,player,game)
            prepared.append((index,uid,config,login,identity))
        def exercise(entry):
            index,uid,config,login,guid=entry
            stamp=1800001010000+index*10000
            def call(name,fields,response):
                nonlocal stamp
                stamp+=1
                return self.call(name,fields,login,stamp,response)
            strength_costs=[int(r['coin']) for r in exp_rows if r['quality']==config['quality'] and int(r['level'])<260]
            self.assertEqual(len(strength_costs),259)
            fields={'guid':guid,'times':5,'cost':[{'id':'coin','num':sum(strength_costs[:5])}]}
            first=call('CSEquipStrengthen',fields,'SCEquipStrengthen')
            self.assertEqual((first.equip.level,first.costTimes),(6,5))
            final_strength=call('CSEquipStrengthenOneKey',{'guid':guid},'SCEquipStrengthenOneKey')
            self.assertEqual((final_strength.equip.level,final_strength.costTimes),(260,254))
            self.assertEqual(sum(c.num for c in final_strength.cost),sum(strength_costs[5:]))
            stamp+=1
            self.assertEqual(self.stamped('CSEquipStrengthenOneKey',{'guid':guid},login,stamp)[0],10001)
            total_refine_exp=sum(int(r['refineExp']) for r in refine_rows if r['quality']==config['quality'])
            stones=math.ceil(total_refine_exp/50)
            fields={'guid':guid,'cost':[{'id':'item_008','num':stones}]}
            refined=call('CSEquipRefine',fields,'SCEquipRefine')
            self.assertEqual((refined.equip.refineLv,refined.equip.refineExp),(80,stones*50-total_refine_exp))
            self.assertEqual(self.call('CSEquipRefine',fields,login,stamp,'SCEquipRefine'),refined)
            stamp+=1
            self.assertEqual(self.stamped('CSEquipRefine',fields,login,stamp)[0],10001)
            costs={'coin':sum(strength_costs),'item_008':stones}
            if int(config['quality'])>=20:
                for level in range(1,6):
                    response=call('CSEquipStar',{'guid':guid},'SCEquipStar')
                    self.assertEqual(response.equip.star,level)
                    original=next(r for r in stars if r['pos']==config['type'] and int(r['starLevel'])==level)
                    exact={part.split(',')[0]:int(part.split(',')[1]) for part in original['cost'].split(';')}
                    self.assertEqual({r.id:r.num for r in response.cost},exact)
                    for key,number in exact.items():costs[key]=costs.get(key,0)+number
                stamp+=1
                self.assertEqual(self.stamped('CSEquipStar',{'guid':guid},login,stamp)[0],10001)
            else:
                stamp+=1
                self.assertEqual(self.stamped('CSEquipStar',{'guid':guid},login,stamp)[0],10001)
            forge_max=15 if config['quality']=='23' else 8 if config['quality']=='20' else 0
            forge_level=0
            attempts=0
            while forge_level<forge_max and attempts<2000:
                row=next(r for r in forge_rows if r['quality']==config['quality'] and int(r['forgeLevel'])==forge_level+1)
                fields={'guid':guid,'cost':[{'id':'frag_'+config['id'],'num':int(row['fragNum'])},{'id':'coin','num':int(row['cost'].split(',')[1])}]}
                response=call('CSEquipForge',fields,'SCEquipForge')
                self.assertIn(response.result,(0,1))
                self.assertEqual(response.equip.forgeLevel,forge_level+response.result)
                self.assertEqual(self.call('CSEquipForge',fields,login,stamp,'SCEquipForge'),response)
                forge_level=response.equip.forgeLevel
                for c in response.cost:costs[c.id]=costs.get(c.id,0)+c.num
                attempts+=1
            self.assertEqual(forge_level,forge_max)
            if forge_max:
                stamp+=1
                self.assertEqual(self.stamped('CSEquipForge',{'guid':guid,'cost':[{'id':'coin','num':1}]},login,stamp)[0],10001)
            player,game=self.profile(login)
            self.assertEqual(player['coin'],1000000000-costs['coin'])
            material_balances={r['id']:r['num'] for r in (game.get('items') or [])}
            for key in resources:self.assertEqual(material_balances.get(key,0),100000-costs.get(key,0))
            if forge_max:self.assertEqual(material_balances.get('frag_'+config['id']),100000-costs['frag_'+config['id']])
            call('CSCopyEnter',{'chapterId':'1','missionId':'1'},'SCCopyEnter')
            base=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0]
            call('CSFormationOn',{'formatPos':1,'id':config['id'],'guid':guid},'SCFormationOn')
            mounted=call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},'SCCopyRunMapInfo').battlePlayer.formation[0]
            self.assertTrue(any(getattr(mounted.prop,k)>getattr(base.prop,k) for k in ('att','hp','pdef','mdef')))
            call('CSFormationOff',{'formatPos':1,'id':config['id'],'guid':guid},'SCFormationOff')
            reset={'guid':guid}
            call('CSEquipReBuild',reset,'SCEquipReBuild')
            final=self.profile(login)
            equip=next(e for e in final[1]['equipment'] if e['guid']==guid)
            self.assertEqual((equip['level'],equip['refineLv'],equip['refineExp'],equip['star']),(1,0,0,0))
            self.assertEqual(final[0]['coin'],1000000000)
            self.assertEqual(final[0]['diamond'],950)
            for r in final[1]['items']:self.assertEqual(r['num'],100000)
            restart.append((uid,reset,stamp,final))
            results.append({'equipmentId':config['id'],'highestStrength':260,'highestRefine':80,'highestLegalStar':5 if int(config['quality'])>=20 else 0,'highestLegalForge':forge_max,'actualForgeAttempts':attempts,
                            'exactOriginalCosts':costs,'retainedRefineExp':stones*50-total_refine_exp,'battleProjection':'passed','actualInvestmentReset':'passed'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(exercise,prepared))
        self.stop();self.start()
        for uid,fields,stamp,saved in sorted(restart):
            login=self.login(uid);self.call('CSEquipReBuild',fields,login,stamp,'SCEquipReBuild');self.assertEqual(self.profile(login),saved)
        (shared.ROOT/'analysis/p5/equipment-highest-lifecycle-validation.json').write_text(json.dumps({
            'date':'2026-10-14','all36Paths':sorted(results,key=lambda r:r['equipmentId']),'originalHTTP':'passed','goInnoDB':domain_result,'restart':'passed',
            'sourceSha256':self.source_shas,'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'baselineSource':self.baseline_source,
            'productionModified':False,'preparation':'dedicated finite original zero-stage gear and balances; all36 real reward constructors in domain; native source acquisition remains separate',
            'remaining':['every original natural equipment and material source','original critical/auto strength stochastic server rules','native phone and dynamic original golden']},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
