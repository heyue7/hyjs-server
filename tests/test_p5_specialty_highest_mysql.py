"""All49 original eligible specialty identities: real five-type100/10 HTTP growth."""
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import unittest
import test_p5_talent_scope_mysql as shared
import test_game_creation_mysql as stressed_transport


@unittest.skipUnless(os.environ.get('HYJS_P5_SPECIALTY_MYSQL_TEST')=='1','Explicit isolated all49 highest specialty test')
class SpecialtyHighestMySQLTests(unittest.TestCase):
    setUpClass=classmethod(shared.TalentScopeMySQLTests.setUpClass.__func__)
    root_sql=classmethod(shared.TalentScopeMySQLTests.root_sql.__func__)
    send=shared.TalentScopeMySQLTests.send
    login=shared.TalentScopeMySQLTests.login
    stamped=stressed_transport.GameCreationMySQLTests.stamped
    call=shared.TalentScopeMySQLTests.call
    profile=shared.TalentScopeMySQLTests.profile
    start=shared.TalentScopeMySQLTests.start
    stop=shared.TalentScopeMySQLTests.stop
    setUp=shared.TalentScopeMySQLTests.setUp
    sql=shared.TalentScopeMySQLTests.sql
    write_profile=shared.TalentScopeMySQLTests.write_profile

    def test_all49_five_specialties100_strength10_actual_costs_reset_restart(self):
        self.start()
        heroes=[r for r in shared.original_table('Hero') if r['quality'] in ('20','23') and r['heroClass'] in ('0','100','101')]
        self.assertEqual(len(heroes),49)
        levels={(int(r['type']),int(r['level'])):r for r in shared.original_table('SkillSpecialty')}
        strengths={(int(r['type']),int(r['strengthLevel'])):r for r in shared.original_table('SkillSpecialtyStrength')}
        prepared=[];results=[];restart=[]
        for index,hero in enumerate(heroes):
            uid='specialty-highest-native-'+hero['id'];login=self.login(uid)
            self.call('CSPlayerCreate',{'playerName':'原专精全最高','heroId':'hero_047' if hero['heroClass']=='101' else 'hero_043'},login,1800001100000+index,'SCPlayerCreate')
            player,game=self.profile(login)
            guid=hero['id']+'-specialty-native'
            player.update(level=100,coin=1000000000,diamond=1000,warriorBagSize=1000,noviceGuideSchedule=8)
            target={'id':hero['id'],'guid':guid,'level':100,'exp':0,'talent':0,'training':{},'tempTraining':{},'breached':{'level':1},'awake':{}}
            if hero['heroClass']=='0':game['warriors'].append(target)
            else:game['warriors']=[target]
            # Cultivation of an unmounted card avoids ranking each intermediate
            # attribute bump. All final maximum effects are mounted and sampled.
            game['formation']=[]
            game['items']=[{'id':f'item_{number:03d}','num':100000} for number in range(439,455)]
            if hero['heroClass']!='0':player['heroWarId']=guid
            self.write_profile(login,player,game);prepared.append((index,uid,hero,login,guid))
        def exercise(entry):
            index,uid,hero,login,guid=entry
            stamp=1800001110000+index*10000
            records={kind:(1,1) for kind in (1,2,3)}
            spent={};mutations=0
            for kind in range(1,6):
                self.assertIn(kind,records,'Original canOpen next type missing')
                level,strength=records[kind]
                while level<100:
                    row=levels[kind,level+1]
                    strengthening=strength<int(row['needStrengthLevel'])
                    request_name='CSStrengthRoleSkillRefineLevel' if strengthening else 'CSUpRoleSkillRefineLevel'
                    response_name='SCStrengthRoleSkillRefineLevel' if strengthening else 'SCUpRoleSkillRefineLevel'
                    original=strengths[kind,strength+1]['strengthCost'] if strengthening else row['levelUpCost']
                    exact={part.split(',')[0]:int(part.split(',')[1]) for part in original.split(';')}
                    fields={'roleGuid':guid,'type':kind};stamp+=1
                    response=self.call(request_name,fields,login,stamp,response_name)
                    self.assertEqual({r.id:r.num for r in response.cost},exact)
                    for key,number in exact.items():spent[key]=spent.get(key,0)+number
                    if strengthening:strength+=1
                    else:level+=1
                    records={r.type:(r.level,r.strengthLevel) for r in response.skillRefine}
                    self.assertEqual(records[kind],(level,strength));mutations+=1
                    if level==100:self.assertEqual(self.call(request_name,fields,login,stamp,response_name),response)
                self.assertEqual((level,strength),(100,10))
                for name in ('CSUpRoleSkillRefineLevel','CSStrengthRoleSkillRefineLevel'):
                    stamp+=1;self.assertEqual(self.stamped(name,{'roleGuid':guid,'type':kind},login,stamp)[0],10001)
            self.assertEqual(mutations,540)
            player,game=self.profile(login)
            target=next(w for w in game['warriors'] if w['guid']==guid)
            self.assertEqual({r['type']:(r['level'],r['strengthLevel']) for r in target['skillRefine']},{k:(100,10) for k in range(1,6)})
            balances={r['id']:r['num'] for r in (game.get('items') or [])}
            for number in range(439,455):
                key=f'item_{number:03d}';self.assertEqual(balances.get(key,0),100000-spent.get(key,0))
            stamp+=1;self.call('CSFormationOn',{'formatPos':1,'id':hero['id'],'guid':guid},login,stamp,'SCFormationOn')
            stamp+=1;self.call('CSCopyEnter',{'chapterId':'1','missionId':'1'},login,stamp,'SCCopyEnter')
            stamp+=1;actor=self.call('CSCopyRunMapInfo',{'chapterId':'1','missionId':'1'},login,stamp,'SCCopyRunMapInfo').battlePlayer.formation[0]
            hp_bonus=sum(int(float(part.split(',')[1])) for row in (levels[1,100],strengths[1,10]) for part in row['propAdd'].split(';') if part.startswith('hp,'))
            self.assertGreaterEqual(actor.prop.hp,hp_bonus)
            self.assertGreater(actor.prop.att,0);self.assertGreater(actor.prop.hp,0)
            reset={'guid':guid}
            if hero['heroClass']=='0':
                stamp+=1;self.call('CSFormationOff',{'formatPos':1,'id':hero['id'],'guid':guid},login,stamp,'SCFormationOff')
                stamp+=1;self.call('CSWarriorReset',reset,login,stamp,'SCWarriorReset')
                saved=self.profile(login)
                self.assertFalse(next(w for w in saved[1]['warriors'] if w['guid']==guid).get('skillRefine'))
                self.assertEqual(saved[0]['coin'],1000000000);self.assertEqual(saved[0]['diamond'],950)
                for key in spent:self.assertEqual(next(r['num'] for r in saved[1]['items'] if r['id']==key),100000)
                restart.append((uid,reset,stamp,saved,False))
            else:
                saved=self.profile(login);stamp+=1
                self.assertEqual(self.stamped('CSWarriorReset',reset,login,stamp)[0],10001)
                self.assertEqual(self.profile(login),saved);restart.append((uid,reset,stamp,saved,True))
            results.append({'heroId':hero['id'],'actualUpgradeMutations':mutations,'highestAll5Types':[100,10],
                            'originalExactCosts':spent,'battleProjection':'passed','reset':'main protected' if hero['heroClass']!='0' else 'all actual resources refunded'})
        with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:list(pool.map(exercise,prepared))
        self.assertEqual(sum(r['actualUpgradeMutations'] for r in results),26460)
        self.stop();self.start()
        for uid,fields,stamp,saved,is_main in sorted(restart):
            login=self.login(uid)
            if is_main:self.assertEqual(self.stamped('CSWarriorReset',fields,login,stamp)[0],10001)
            else:self.call('CSWarriorReset',fields,login,stamp,'SCWarriorReset')
            current=self.profile(login)
            for key in ('energy','stamina'):self.assertGreaterEqual(current[0].get(key,0),saved[0].get(key,0))
            normalize=lambda p:[{k:v for k,v in p[0].items() if k not in ('energy','stamina','energyLastRefreshTime','staminaLastRefreshTime')},p[1]]
            self.assertEqual(normalize(current),normalize(saved))
        (shared.ROOT/'analysis/p5/specialty-highest-lifecycle-validation.json').write_text(json.dumps({
            'date':'2026-10-14','originalEligibleIdentities':49,'actualUpgradeMutations':26460,'all49Paths':sorted(results,key=lambda r:r['heroId']),
            'originalHTTP':'passed','isolatedMySQL':'passed','processRestart':'passed','sourceSha256':self.source_shas,
            'binarySha256':hashlib.sha256(Path(self.binary).read_bytes()).hexdigest(),'productionModified':False,
            'preparation':'finite isolated level100 prerequisites, zero-specialty original identities and original items439..454; natural sources remain separate',
            'coldReplayTimerBoundary':'only normal energy/stamina regeneration and refresh timestamps may advance; full inventory, costs and every other Player/Game field exact',
            'unlockPolicy':'existing SQL-declared initial types1/2/3 and dual-strength canOpen policy; unknown original server initialization stays audited',
            'remaining':['every original natural skill material and card source','original server initialization policy audit','native phone','dynamic original golden']},ensure_ascii=False,indent=2)+'\n')


if __name__=='__main__':unittest.main()
