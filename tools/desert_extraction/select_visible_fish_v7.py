"""Select four joint fish candidates from the pinned Unity 5.3 v7 audit.

This generates only an unapproved input for the next scene audit. The original
12-fish native 3D range, inferred map boundary, and spacing are not relaxed.
"""
import argparse,hashlib,itertools,json,math
from pathlib import Path
from tools.desert_extraction.select_map_candidates import distance,inside

PIN={
 'prior':'5ac60121898949f73435edf2315fa6c789fc40fae7c8c3d8c46bb47de5152277',
 'audit':'bf2176f27e55b5c0329553617b9861ec19485e36b70149332dbcac2952ff53dc',
 'options':'744ed141fb0098ee3e416f2c0f8070ee616e63bf40d37dced99acbf57f6ab102',
 'navigation':'70ff1c25d3e065b9038d1a62bca3123e8eb3c83025a7e2d2980f3bdf10a8107e',
}
BUNDLE='2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1'
def read(path,key):
 raw=path.read_bytes()
 if hashlib.sha256(raw).hexdigest()!=PIN[key]: raise ValueError('Unexpected '+key+' audit SHA')
 return json.loads(raw)
def fail_if(test,message):
 if test: raise ValueError(message)
def dist3(a,b): return math.sqrt(sum((a[k]-b[k])**2 for k in ('x','y','z')))
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for name in ('prior','audit','options','navigation','output'):p.add_argument('--'+name,type=Path,required=True)
 a=p.parse_args();fail_if(a.output.exists(),'Output exists')
 prior,audit,options,nav=(read(getattr(a,k),k) for k in ('prior','audit','options','navigation'))
 fail_if(prior['schema']!='hyjs-desert-v4-selected-audit-input-v1' or len(prior['points'])!=59 or prior['eligibleForRelease'],'Unexpected prior')
 fail_if(audit['schema']!='hyjs-desert-v4-selected-audit-report-v1' or audit['automaticPassCount']!=59 or audit['pointCount']!=59 or audit['allEligibleForRelease'],'Unexpected r5 audit')
 fail_if(options['schema']!='hyjs-desert-v4-visible-fish-options-v7' or options['eligibleForRelease'] or options['navVertices']!=4947 or options['navTriangles']!=2337,'Unexpected v7 audit')
 for r in (prior,audit,options):
  fail_if(r['bundleSha256']!=BUNDLE or any(r[k]!=prior[k] for k in ('designSha256','minimapSha256')),'Scene provenance differs')
 before={p['id']:p for p in prior['points']}; inspected={p['id']:p for p in audit['points']}
 fail_if(len(before)!=59 or set(before)!=set(inspected),'Audit point identities differ')
 for key,p in before.items():
  q=inspected[key];fail_if(q['kind']!=p['kind'] or not q['automatedChecksPass'] or q['eligibleForRelease'] or dist3(p['modelPosition'],q['modelPosition'])>1e-5 or dist3(p['standPosition'],q['standPosition'])>1e-5,'Prior not verified: '+key)
 ids=('F01','F02','F03','F04'); rows={p['id']:p for p in options['points']}
 fail_if(set(rows)!=set(ids),'Candidate identities differ')
 fish=[p for p in prior['points'] if p['kind']=='fish'];fixed=[p for p in fish if p['id'] not in ids]
 fail_if(len(fish)!=12 or len(fixed)!=8 or len(nav['outer'])<3,'Unexpected fixed fish or boundary')
 choices={}
 for key in ids:
  row=rows[key];fail_if(row['waterName']!='Western channel','Water identity differs')
  values=[];orig=inspected[key]['originalPlan']
  for index,c in enumerate(row['candidates']):
   model,stand=c['modelPosition'],c['standPosition']; r=dist3(model,stand); displacement=distance(model,orig)
   if (c['eligibleForRelease'] or not c['lineClear'] or not math.isfinite(r) or r>=3.8 or displacement>20 or
       abs(r-c['standDistance'])>1e-4 or abs(displacement-c['modelDisplacement'])>1e-4 or
       abs(c['viewportX'])>.9 or abs(c['viewportY'])>.9 or not inside(nav['outer'],model) or not inside(nav['outer'],stand) or
       any(distance(model,q['modelPosition'])<2.5 for q in fixed)):
    continue
   values.append((index,c))
  fail_if(not values,'No valid candidates for '+key); choices[key]=values
 best=None;selected=None;count=0
 for seq in itertools.product(*(choices[k] for k in ids)):
  cs=[c for _,c in seq]
  if any(distance(x['modelPosition'],y['modelPosition'])<2.5 for i,x in enumerate(cs) for y in cs[i+1:]):continue
  count+=1
  score=(sum(c['standDistance']>=3.5 for c in cs),max(max(abs(c['viewportX']),abs(c['viewportY'])) for c in cs),
         sum(abs(c['viewportX'])+abs(c['viewportY']) for c in cs),sum(c['modelDisplacement'] for c in cs))
  if best is None or score<best:best,selected=score,seq
 fail_if(selected is None,'No joint 2.5m-separated fish candidates')
 chosen=dict(zip(ids,selected)); result=[]
 for point in prior['points']:
  row=dict(point)
  if point['id'] in chosen:
   index,c=chosen[point['id']]; row.update(modelPosition=c['modelPosition'],standPosition=c['standPosition'],source='visible-fish-v7',
     standDistance=c['standDistance'],modelDisplacement=c['modelDisplacement'],candidateIndex=index,
     viewportX=c['viewportX'],viewportY=c['viewportY'],cameraAngle=c['cameraAngle'],lineClear=c['lineClear'])
  row['eligibleForRelease']=False;result.append(row)
 all_fish=[p for p in result if p['kind']=='fish']; pairs=[(distance(x['modelPosition'],y['modelPosition']),x['id'],y['id']) for i,x in enumerate(all_fish) for y in all_fish[i+1:]]
 minimum=min(pairs); fail_if(minimum[0]<2.5,'Selected fish too close')
 report=dict(prior);report.update(points=result,sourceSHA256={**prior['sourceSHA256'],'r5Input':PIN['prior'],'r5Audit':PIN['audit'],'visibleFishV7':PIN['options'],'navigation':PIN['navigation']},
   minimumFishHorizontalSpacing=minimum[0],minimumFishPair=list(minimum[1:]),eligibleForRelease=False,
   lowMarginFish=[p['id'] for p in all_fish if p['standDistance']>=3.5],jointCandidateCombinations=count)
 report['manualGates']=list(prior['manualGates'])+['camera body visibility and bridge occlusion at F01-F04','player view and cactus occlusion at F09']
 a.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps({'sha256':hashlib.sha256(a.output.read_bytes()).hexdigest(),'count':len(result),'combinations':count,'minFishSpacing':minimum,'lowMarginFish':report['lowMarginFish'],
  'chosen':{k:{'candidateIndex':chosen[k][0],'standDistance':chosen[k][1]['standDistance'],'viewport':[chosen[k][1]['viewportX'],chosen[k][1]['viewportY']]} for k in ids},'eligibleForRelease':False}))
if __name__=='__main__':main()
