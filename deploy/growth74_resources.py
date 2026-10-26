"""Generate shared original-format item rows and bounded monthly award packages."""
import json,pathlib
ROOT=pathlib.Path(__file__).resolve().parents[1];data=ROOT/'hyjs-server/service/data';p=json.loads((data/'growth74_parameters.json').read_text())
items=[];uses=[]
def item(id,name,cat,desc,icon='item_icon_050',quality='5'):
 items.append(dict(itemId=id,name=name,star=quality,category=cat,icon=icon,desc=desc,intro=desc))
def use(id,item,result,odds,cat):uses.append(dict(id=str(id),itemId=item,result=result,odds=str(odds),category=cat))
item('item_9760','OS角色自选箱','5','可选择一名OS角色：OS桐人、OS亚丝娜、OS诗乃。','item_icon_053','6')
item('item_9761','王者角色自选箱','5','可选择一名王者角色：双刀桐人、ALO桐人、亚丝娜、诗乃。','item_icon_053','6')
item('item_9762','经验水晶补给箱','2','内含高级经验水晶5个。');item('item_9763','饰品经验补给箱','2','内含高级经验饰品50个。')
for b,k,ids in [(976000,'item_9760',['hero_077','hero_078','hero_079']),(976010,'item_9761',['hero_001','hero_002','hero_003','hero_005'])]:
 for i,t in enumerate(ids):use(b+i,k,t+',1',i+1,'5')
use(976020,'item_9762','hero_exp_004,5',100,'2');use(976021,'item_9763','trea_010,50',100,'2')
def packed(d):
 out={}
 for k,n in d.items():
  k={'NEW_OS_SELECTOR':'item_9760','NEW_KING_SELECTOR':'item_9761'}.get(k,k)
  if k=='hero_exp_004'and n>100:out['item_9762']=n//5;n%=5
  if k=='trea_010'and n>100:out['item_9763']=n//50;n%=50
  if n:out[k]=n
 return out
monthly=[]
special={'item_119','item_339','item_9760','item_9761','item_067'}
for i,t in enumerate(p['monthly_tiers']):
 r=packed(t['increment']);rare={k:v for k,v in r.items()if k in special};ordinary=[(k,v)for k,v in r.items()if k not in special]
 for j,start in enumerate(range(0,len(ordinary),70)):
  id=f'item_{9770+i*2+j}';item(id,f'盛典{t["threshold"]}档养成补给'+('·下'if j else '·上'),'2','打开获得本档养成资源。')
  use(977000+i*2+j,id,';'.join(k+','+str(v)for k,v in ordinary[start:start+70]),100,'2');rare[id]=1
 monthly.append(dict(threshold=t['threshold'],increment=rare))
(data/'growth74_resources.json').write_text(json.dumps(dict(items=items,uses=uses,monthlyTiers=monthly),ensure_ascii=False,separators=(',',':'))+'\n');print(len(items),'shared item definitions')
