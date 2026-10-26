import base64, contextlib, copy, importlib.util, io, json, subprocess, unittest
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('activity_patch75', ROOT/'deploy/growth75_activity_patch.py')
p=importlib.util.module_from_spec(spec);spec.loader.exec_module(p)

class Activity75(unittest.TestCase):
 def test_mysql_export_retains_numeric_prize_identity(self):
  stream=io.StringIO()
  with contextlib.redirect_stdout(stream):p.export_sql()
  line=next(v for v in stream.getvalue().splitlines() if v.startswith("SELECT 'gameplay_pool_rewards',"))
  self.assertIn("'id',`id`",line)
  self.assertNotIn('CONVERT(`id` USING utf8mb4)',line)
  data=json.loads((ROOT/'hyjs-server/service/data/growth74_activities.json').read_text())
  form=next(copy.deepcopy(v['form']) for v in data if v['form']['mode']=='turntable')
  tables={k:[] for k in p.TABLES}
  tables['admin_activity_occurrences']=[dict(id='typed-wheel',consumer_key='typed-wheel',form_json=p.compact(form))]
  prize=form['prizes'][0]
  reward=dict(pool_id='typed-wheel',id=prize['id'],resource_id=prize['resourceId'],amount=prize['amount']+1,weight=prize['weight'],stock=prize['stock'],guaranteed=0)
  tables['gameplay_pool_rewards']=[reward]
  changes=p.make_plan(tables)
  self.assertEqual(len(changes),1)
  self.assertEqual(changes[0]['key'],dict(pool_id='typed-wheel',id=prize['id']))
  self.assertEqual(changes[0]['after'],{'amount':prize['amount']})
  reward['id']=str(prize['id'])
  with self.assertRaisesRegex(ValueError,'integer SQL type'):p.make_plan(tables)
  reward['id']=999
  with self.assertRaisesRegex(ValueError,'absent from reviewed form'):p.make_plan(tables)
 def test_source_rewards_and_wheels(self):
  data=json.loads((ROOT/'hyjs-server/service/data/growth74_activities.json').read_text())
  self.assertFalse(any(a['form']['mode']=='limited-shop' for a in data))
  for a in data:
   f=a['form']
   for m in f.get('milestones',[]):
    self.assertTrue(m['rewards'])
    if f['mode'] in ('recharge-total','diamond-spent'):
     for r in m['rewards']:
      if r['resourceId'].startswith('frag_'):self.assertLessEqual(r['amount'],10)
      if r['resourceId'].startswith('item_') and r['resourceId'][5:].isdigit():self.assertFalse(9770<=int(r['resourceId'][5:])<=9787)
   if f['mode']=='turntable':
    gems=[r for r in f['prizes'] if r['resourceId']=='MineItem_079']
    self.assertEqual([(r['amount'],r['stock']) for r in gems],[(4,4)])
    self.assertEqual(len(f['prizes']),9)
    self.assertEqual(p.adjust_form(f),f)
   if f['mode']=='login-total' and a['timing']=='character-day':
    self.assertEqual(a['startDay'],1);self.assertEqual([m['goal'] for m in f['milestones']],[1])
   if a['durationDays']==1 and f['mode'] in ('arena-wins','wanted-attempts','copy-win'):
    self.assertIn({'resourceId':'item_374','amount':200},f['milestones'][-1]['rewards'])
 def test_copy_weight_and_frequency(self):
  tables=json.loads((ROOT/'hyjs-server/service/data/growth74_table_rows.json').read_text())
  for r in tables['DailyCopy']['rows']:self.assertEqual(r['Count'],'2')
  for r in tables['DailyCopyChapter']['rows']:
   if r['Drop1']:
    for part in r['Drop1'].split('|'):self.assertEqual(sum(int(x.split(',')[1]) for x in part.split(':')[1].split(';')),1000)
 def test_go_base64_frozen_milestones(self):
  old=[{'goal':1,'rewards':'equip_025,1'},{'goal':3,'rewards':'equip_025,1'}]
  obj={'id':'role-orange','goal':1,'rewards':'equip_025,1','milestones':base64.b64encode(p.compact(old).encode()).decode()}
  new=json.loads(p.adjust_period(p.compact(obj)))
  self.assertEqual(json.loads(base64.b64decode(new['milestones'])),old[:1])
  self.assertEqual(json.loads(p.adjust_period(p.compact(new))),new)
 def test_empty_monthly_box_keeps_valid_small_tier(self):
  f={'mode':'recharge-total','milestones':[{'goal':600,'rewards':[{'resourceId':'item_9770','amount':1}]}]}
  self.assertEqual(p.adjust_form(f)['milestones'][0]['rewards'],[{'resourceId':'coin','amount':20000}])
 def test_single_tier_live_and_frozen_reward_cap(self):
  cfg=dict(id='weekly-pet',source='成长投放V12',enabled=True,goal=1288,rewards='frag_pet_078,600',milestones=None)
  frozen=p.compact(cfg)
  t={k:[] for k in p.TABLES}
  t['p1_period_instances']=[dict(cfg,frozen=frozen)]
  t['p1_period_progress']=[dict(instance_id='weekly-pet',player_id=7,snapshot=frozen)]
  changes=p.make_plan(t)
  self.assertEqual(len(changes),2)
  live=changes[0]['after'];self.assertEqual(live['rewards'],'frag_pet_078,10');self.assertNotIn('milestones',live)
  self.assertEqual(json.loads(live['frozen'])['rewards'],'frag_pet_078,10')
  self.assertIsNone(json.loads(live['frozen'])['milestones'])
  self.assertEqual(json.loads(changes[1]['after']['snapshot'])['rewards'],'frag_pet_078,10')
  for c in changes:
   table=c['table'];row=t[table][0];row.update(c['after'])
  self.assertEqual(p.make_plan(t),[])
 def test_weekly_sacred_two_boxes_become_four_small_fragment_rewards(self):
  old_rewards='item_9760,1;item_9761,1;item_119,2'
  expected='item_9760,1;item_9761,1;frag_equip_037,10;frag_equip_038,10;frag_equip_039,10;frag_equip_040,10'
  self.assertEqual(p.adjust_spec(old_rewards),expected)
  self.assertEqual(p.adjust_spec(expected),expected)
  form={'mode':'recharge-total','name':'每周角色庆典','milestones':[{'goal':128800,'rewards':[{'resourceId':k,'amount':n} for k,n in p.resources(old_rewards).items()]}]}
  adjusted=p.adjust_form(form)
  self.assertEqual(adjusted['milestones'][0]['rewards'],[{'resourceId':k,'amount':n} for k,n in p.resources(expected).items()])
  self.assertEqual(p.adjust_form(adjusted),adjusted)
  for packed in (False,True):
   cfg=dict(id='weekly-sacred',source='成长投放V12',enabled=True,goal=128800,rewards=old_rewards,milestones=base64.b64encode(p.compact([dict(goal=128800,rewards=old_rewards)]).encode()).decode() if packed else None)
   t={k:[] for k in p.TABLES};t['p1_period_instances']=[dict(cfg,milestones=p.compact([dict(goal=128800,rewards=old_rewards)]) if packed else None,frozen=p.compact(cfg))]
   t['p1_period_progress']=[dict(instance_id='weekly-sacred',player_id=7,snapshot=p.compact(cfg))]
   changes=p.make_plan(t);self.assertEqual(len(changes),2)
   for c in changes:
    obj=t[c['table']][0];obj.update(c['after'])
   live=t['p1_period_instances'][0];self.assertEqual(live['rewards'],expected)
   self.assertEqual(json.loads(live['frozen'])['rewards'],expected)
   self.assertEqual(json.loads(t['p1_period_progress'][0]['snapshot'])['rewards'],expected)
   self.assertEqual(p.make_plan(t),[])
  data=json.loads((ROOT/'hyjs-server/service/data/growth74_activities.json').read_text())
  weekly=[a['form'] for a in data if a['form'].get('name')=='每周角色庆典']
  self.assertEqual(len(weekly),26)
  for f in weekly:
   rewards=next(m['rewards'] for m in f['milestones'] if m['goal']==128800)
   self.assertNotIn('item_119',[r['resourceId'] for r in rewards])
   for n in range(37,41):self.assertIn({'resourceId':f'frag_equip_{n:03d}','amount':10},rewards)

 def test_current_copy_quota_never_erases_completed_runs(self):
  t={k:[] for k in p.TABLES};t['daily_copy_states']=[dict(player_id=1,day='2026-10-18',type='PetEquip',used=3,limit=3),dict(player_id=2,day='2026-10-18',type='PetEquip',used=1,limit=1)]
  changes=p.make_plan(t);self.assertEqual(len(changes),1);self.assertEqual(changes[0]['key']['player_id'],2);self.assertEqual(changes[0]['after'],{'limit':2})
  t['daily_copy_states'][1]['limit']=2;self.assertEqual(p.make_plan(t),[])

if __name__=='__main__':unittest.main()
