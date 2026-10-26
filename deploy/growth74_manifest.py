"""Build the authorized V12 forms for existing activity consumers. No DB/network writes."""
import json, pathlib
ROOT=pathlib.Path(__file__).resolve().parents[1]
p=json.loads((ROOT/'hyjs-server/service/data/growth74_parameters.json').read_text())
monthly=json.loads((ROOT/'hyjs-server/service/data/growth74_resources.json').read_text())['monthlyTiers']
def rewards(d):
 out=[]
 for k,n in d.items():
  if not n:continue
  k={'NEW_OS_SELECTOR':'item_9760','NEW_KING_SELECTOR':'item_9761'}.get(k,k)
  if k=='hero_exp_004' and n>100:
   out.append({'resourceId':'item_9762','amount':n//5});n%=5
  if k=='trea_010' and n>100:
   out.append({'resourceId':'item_9763','amount':n//50});n%=50
  if n:out.append({'resourceId':k,'amount':n})
 return out
items=[]
def add(name,mode,start,duration,ms=None,products=None,options=None,prizes=None,personal=False,lane=0):
 f={'mode':mode,'templateId':mode+'-original','name':name,'source':'成长投放V12 / 2026-10-26','alias':'成长投放V12','repeatPolicy':'repeat','products':products or [],'milestones':ms or [],'prizes':prizes or [],'options':options or {}}
 items.append(dict(timing='character-day'if personal else 'calendar',startDay=start,durationDays=duration,claimDays=0,lane=lane,form=f))
def m(goal,d):return dict(goal=goal,rewards=rewards(d))
def prod(name,d,price,index=1):return dict(id=str(index),name=name,index=index,rewards=rewards(d),cost=dict(resourceId='diamond',amount=price),dailyLimit=1,totalLimit=1,stock=-1,minLevel=1,minVip=0,discountBps=10000,weight=1)
# Opening offers now last at least fourteen days; independently earned cumulative milestones.
add('启程庆典','recharge-total',1,14,[m(x['threshold']*100,x['increment'])for x in p['personal_tiers']],personal=True,lane=1)
add('启程装备补给','login-total',1,28,[m(day,{f'equip_{i:03}':1 for i in range(25,29)})for day in [1,3,7,14,21]],personal=True,lane=2)
add('料理研习礼','login-total',30,14,[m(1,{'item_065':1})],personal=True,lane=2)
add('旅途纪念礼','login-total',180,14,[m(1,{'item_9760':1,'item_9761':1})],personal=True,lane=2)
for day in range(1,181):
 add('每日竞技挑战','arena-wins',day,1,[m(1,{'coin':5000000,'popularity':600}),m(3,{'coin':5000000,'popularity':400})],lane=3)
 add('每日悬赏挑战','wanted-attempts',day,1,[m(2,{'coin':4000000,'score':2100}),m(4,{'coin':10000000}),m(6,{'coin':6000000,'score':900})],lane=4)
 add('每日前线补给','copy-win',day,1,[m(10,{'light':170})],lane=5)
for week,day in enumerate(range(1,181,7)):
 add('每周活跃礼','login-total',day,7,[m(3,{'item_9711':288})],lane=6)
 add('每周试炼挑战','trial-stars',day,7,[m(27,{'coin':60000000})],lane=7)
 add('每周成长回馈','diamond-spent',day,7,[m(x['threshold'],x['increment']) for x in p['consumption_return']['tier_rotation_4weeks'][week%4]],lane=8)
 add('每周角色庆典','recharge-total',day,7,[m(128800,p['weekly_recharge_rewards']['rewards'])],lane=9)
 # Preserve approved condition bundles; rewards sharing a mode are grouped into one template.
 groups={}
 for x in p['weekly_core_themes']+p['weekly_extra_diamond_offers']:
  if x.get('condition'):
   c=x['condition'];goals=c.get('goals',[c.get('goal',1)])
   for goal in goals:
    bucket=groups.setdefault(c['mode'],{}).setdefault(goal,{})
    for k,n in x['rewards'].items():bucket[k]=bucket.get(k,0)+n
 for mode,goals in groups.items():add({'arena-wins':'每周竞技成长','trial-stars':'每周试炼成长','wanted-attempts':'每周悬赏成长'}[mode],mode,day,7,[m(g,d)for g,d in sorted(goals.items())],lane=10)
 buy=[prod(x['name'],x['rewards'],x['price'],i+1) for i,x in enumerate(p['weekly_extra_diamond_offers'])if x.get('price',0)>0]
 if buy:add('每周养成储备','limited-shop',day,7,products=buy,lane=11)
 # Native trial shop: each week has its own original catalogue and total limits.
 add('试炼商店','shop-window',day,7,options={'shopKey':'point'},lane=12)
# Fourteen-day personal offers retain the weekly economic rate with twice the resources and price.
for day in range(1,181,14):
 buy=[]
 for i,x in enumerate(p['weekly_core_themes']):
  if x.get('kind')=='limited_event_purchase':buy.append(prod(x['name'],{k:n*2 for k,n in x['rewards'].items()},x['price']*2,i+1))
 if buy:add('精灵与阵法研修','limited-shop',day,14,products=buy,personal=True,lane=13)
for phase,day in enumerate(range(1,181,14),1):
 pool=p['wheel']['pools'][min(phase,3)-1]
 prizes=[dict(id=x['id'],resourceId=x['resourceID'],amount=x['amount'],weight=x['weight'],stock=x['stock'],guaranteed=False)for x in pool['rewards']]
 if pool['phase']==1:
  prizes[6]['resourceId']='item_9762';prizes[6]['weight']=400;prizes[8]['weight']=7600
 add('幸运大转盘','turntable',day,14,options=dict(prizePityRules=pool['prizePityRules'],scorePerDraw=1,initialDiamondPool=0,jackpotPerMillion=0,poolContributionBps=0,advancePlaces=0),prizes=prizes,personal=True,lane=14)
# Monthly calendar dates are resolved from the real server opening day by the deployment service.
for month in range(7):
 add('公会商店','shop-window',1,31,options={'shopKey':'guild'},lane=17);items[-1]['monthOffset']=month
for month in range(7):add('月度盛典','recharge-total',1,3,[m(x['threshold']*100,x['increment'])for x in monthly],lane=15);items[-1]['monthOffset']=month
out=ROOT/'hyjs-server/service/data/growth74_activities.json';out.write_text(json.dumps(items,ensure_ascii=False,separators=(',',':'))+'\n');print(len(items),'existing-consumer activity occurrences')
if __name__=='__main__':pass
