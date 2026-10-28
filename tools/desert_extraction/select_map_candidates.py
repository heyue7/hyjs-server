"""Jointly select non-release V4 map candidates for another Unity audit.

Only verified audit report identities are consumed. This script never writes a
server configuration or treats a candidate as an approved deployment point.
"""
import argparse,hashlib,json,math
from itertools import product
from pathlib import Path

PIN={
 'r1':'c5e63ed18032af15c0f6fda3a6ea1ebbff8306b99a977a5d18c31cf0ecb647b3',
 'v2':'7fda61fd3edd0d83fc5126b61b94b51de34507e85ff1945ad9b6a2c4d3ada068',
 'fish':'30ba5ef8f3110da8841627ff3c79df76d6aae5384285babe549059ce02eb667b',
 'f09':'483e89b7a10039b4a089bc8a9a2fc404b0bf60733f0249a8d43950b1ae4f4c68',
 'west':'8d0019aa67ae55138c7155e4fa58d1f0f9e9c1d654191c00540a39ed6a786097',
 'boss':'9324cd12fc03e4107255ea823371425c5d3f8c1a6a74b56d0f7dbe3a703fac5d',
 'navigation':'70ff1c25d3e065b9038d1a62bca3123e8eb3c83025a7e2d2980f3bdf10a8107e',
}
BUNDLE='2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1'
def read(path,pin=None):
 raw=path.read_bytes(); digest=hashlib.sha256(raw).hexdigest()
 if pin and digest!=pin: raise ValueError('Unrecognized audit '+str(path))
 return json.loads(raw),digest
def distance(a,b): return math.hypot(a['x']-b['x'],a['z']-b['z'])
def inside(poly,p):
 result=False; j=len(poly)-1
 for i,a in enumerate(poly):
  b=poly[j]
  if (a['z']>p['z'])!=(b['z']>p['z']) and p['x']<(b['x']-a['x'])*(p['z']-a['z'])/(b['z']-a['z'])+a['x']: result=not result
  j=i
 return result
def components(options,limit):
 keys=sorted(options); links={x:set() for x in keys}
 for i,left in enumerate(keys):
  for right in keys[i+1:]:
   if any(distance(a['modelPosition'],b['modelPosition'])<limit for a in options[left] for b in options[right]):
    links[left].add(right); links[right].add(left)
 result=[]; pending=set(keys)
 while pending:
  stack=[min(pending)]; group=set()
  while stack:
   p=stack.pop()
   if p in group: continue
   group.add(p); stack.extend(links[p]-group)
  pending-=group; result.append(sorted(group,key=lambda p:(len(options[p]),p)))
 return result

def choose(options,minimum):
 chosen={}
 for group in components(options,minimum):
  if math.prod(len(options[x]) for x in group)>3000000: raise ValueError('Too many candidate combinations; refine input')
  best=None; winner=None
  for sequence in product(*(options[k] for k in group)):
   if any(distance(a['modelPosition'],b['modelPosition'])<minimum for i,a in enumerate(sequence) for b in sequence[i+1:]): continue
   # First minimize lost 3D margin, then model displacement, then total range.
   score=(sum(max(0,c['standDistance']-3.5)**2 for c in sequence),sum(c['modelDisplacement'] for c in sequence),sum(c['standDistance'] for c in sequence))
   if best is None or score<best: best=score; winner=sequence
  if winner is None: raise ValueError('No separated candidates for '+','.join(group))
  chosen.update(zip(group,winner))
 return chosen

def main():
 p=argparse.ArgumentParser(description=__doc__)
 for name in ('input','r1','v2','fish','f09','west','boss','navigation','output'): p.add_argument('--'+name,type=Path,required=True)
 p.add_argument('--min-spacing',type=float,default=2.5)
 args=p.parse_args()
 if args.output.exists(): raise ValueError('Output must be new')
 reports={}; hashes={}
 for key in ('input','r1','v2','fish','f09','west','boss','navigation'):
  reports[key],hashes[key]=read(getattr(args,key),PIN.get(key))
 ref=reports['input']
 if ref['schema']!='hyjs-desert-v4-candidate-audit-input-v1' or len(ref['points'])!=59 or ref['bundleSha256']!=BUNDLE: raise ValueError('Unexpected candidate plan')
 for key in ('r1','v2','fish','f09','west'):
  r=reports[key]
  for k in ('designSha256','minimapSha256','bundleSha256'):
   if r[k]!=ref[k]: raise ValueError('Audit provenance mismatch: '+key+' '+k)
 if reports['r1']['schema']!='hyjs-desert-v4-point-audit-v1' or reports['fish']['schema']!='hyjs-desert-v4-fish-options-v4' or reports['f09']['schema']!='hyjs-desert-v4-f09-suggestions-v3': raise ValueError('Unexpected audit schema')
 base={x['id']:x for x in reports['r1']['points']}; repairs={x['id']:x for x in reports['v2']['points']}
 choices={x['id']:x['candidates'] for x in reports['fish']['points']}
 choices['F09']=reports['f09']['candidates']
 for row in reports['west']['points']:
  if row['id'] not in ('F01','F02'): raise ValueError('Unexpected west target')
  choices[row['id']]=row['candidates']
 if set(choices)!={'F%02d'%i for i in range(1,13)} or any(not v for v in choices.values()): raise ValueError('Fish candidates incomplete')
 if any(c['standDistance']>=4 or c['eligibleForRelease'] for values in choices.values() for c in values): raise ValueError('Invalid fish candidate')
 outer=reports['navigation'].get('outer')
 if not isinstance(outer,list) or len(outer)<3: raise ValueError('Invalid fixed outer boundary')
 choices={key:[c for c in values if inside(outer,c['modelPosition']) and inside(outer,c['standPosition'])] for key,values in choices.items()}
 if any(not values for values in choices.values()): raise ValueError('No fish candidate inside fixed boundary')
 if not 2.5<=args.min_spacing<=10: raise ValueError('Fish spacing outside audit bounds')
 selected=choose(choices,args.min_spacing); result=[]
 boss=reports['boss']
 if boss['schema']!='hyjs-desert-v4-boss-search-v1' or boss['navVertices']!=4947 or boss['navTriangles']!=2337: raise ValueError('Unexpected boss audit')
 permitted=[c for c in boss['candidates'] if c['allFour'] and c['displacement']<=6 and all(distance(c['position'],x['modelPosition'])>=3 for x in selected.values())]
 if not permitted: raise ValueError('No four-route boss candidate clear of selected fish')
 bossChoice=min(permitted,key=lambda c:c['displacement'])
 for orig in ref['points']:
  oid,kind=orig['id'],orig['kind']; observed=base[oid]
  if kind=='fish':
   row=selected[oid]; model,stand=row['modelPosition'],row['standPosition']; source='west-fish-v5' if oid in ('F01','F02') else 'fish-v4' if oid!='F09' else 'f09-v3'
  elif kind=='boss':
   model=stand=bossChoice['position']; row=None; source='boss-search-v1'
  elif not observed['automatedChecksPass']:
   candidate=repairs[oid]['candidates']
   if not candidate: raise ValueError('No ground repair: '+oid)
   row=min(candidate,key=lambda c:c['modelDisplacement']); model,stand=row['modelPosition'],row['standPosition']; source='ground-v2'
  else:
   if not observed['hasProjection']: raise ValueError('No original projection: '+oid)
   model=stand=observed['projected']; row=None; source='r1-projection'
  result.append(dict(id=oid,kind=kind,pixelX=orig['pixelX'],pixelY=orig['pixelY'],modelPosition=model,standPosition=stand,source=source,eligibleForRelease=False,
    standDistance=row['standDistance'] if row else 0.0,modelDisplacement=row['modelDisplacement'] if row else bossChoice['displacement'] if kind=='boss' else observed['horizontalShift']))
 fish=[x for x in result if x['kind']=='fish']; pairs=[(distance(a['modelPosition'],b['modelPosition']),a['id'],b['id']) for i,a in enumerate(fish) for b in fish[i+1:]]
 minimum=min(pairs)
 report=dict(schema='hyjs-desert-v4-selected-audit-input-v1',designSha256=ref['designSha256'],minimapSha256=ref['minimapSha256'],bundleSha256=BUNDLE,
  sourceSHA256=hashes,eligibleForRelease=False,minimumFishHorizontalSpacing=minimum[0],minimumFishPair=list(minimum[1:]),
  lowMarginFish=[x['id'] for x in fish if x['standDistance']>=3.5],
  manualGates=['same-water visual check','native actor pivot and 3D interaction on device','standPosition reachability','actual resource model geometry','bridge four mobs','boss four bypass routes','safe NPC positions','server authorization'],points=result)
 args.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps(dict(count=len(result),minimumFishSpacing=minimum,lowMarginFish=report['lowMarginFish'],eligibleForRelease=False),ensure_ascii=False))
if __name__=='__main__': main()
