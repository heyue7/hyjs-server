"""Align two unapproved exit candidates with audited NPC positions."""
import argparse,hashlib,json,math
from pathlib import Path
from tools.desert_extraction.select_map_candidates import distance,inside
PIN={'prior':'e4f404e8387f73655991d6610729a6ac3d2d7a8d6ef74c0a267f21d36416e1f1','stage':'8eca4d81f62abbccf121fb13efbc33c80338208ab5e9d66e5922d253b5a46ca8','navigation':'70ff1c25d3e065b9038d1a62bca3123e8eb3c83025a7e2d2980f3bdf10a8107e','audit':'bf2176f27e55b5c0329553617b9861ec19485e36b70149332dbcac2952ff53dc'}
def read(path,key):
 raw=path.read_bytes()
 if hashlib.sha256(raw).hexdigest()!=PIN[key]:raise ValueError('Unrecognized '+key+' input')
 return json.loads(raw)
def dist3(a,b):return math.sqrt(sum((a[k]-b[k])**2 for k in ('x','y','z')))
def main():
 p=argparse.ArgumentParser(description=__doc__)
 for name in ('prior','stage','navigation','audit','output'):p.add_argument('--'+name,type=Path,required=True)
 a=p.parse_args()
 if a.output.exists():raise ValueError('Output exists')
 prior,stage,nav,audit=(read(getattr(a,k),k) for k in ('prior','stage','navigation','audit'))
 if prior['schema']!='hyjs-desert-v4-selected-audit-input-v1' or len(prior['points'])!=59 or prior['eligibleForRelease']:raise ValueError('Invalid prior')
 if stage['schema']!='hyjs-desert-v4-stage-qa-v2' or stage['eligibleForRelease'] or stage['unityVersion']!='5.3.3p3' or stage['navVertices']!=4947 or stage['navTriangles']!=2337:raise ValueError('Invalid stage')
 rows={p['id']:p for p in prior['points']}; old={p['id']:p for p in audit['points']}; npcs={p['id']:p for p in stage['npcs']}
 if len(rows)!=59 or set(('S','E1','E2','E3'))-set(rows) or dist3(stage['anchor'],rows['S']['modelPosition'])>1e-5:raise ValueError('Stage anchor differs')
 if set(('exit_E1','exit_E2','exit_E3'))-set(npcs):raise ValueError('Stage exits incomplete')
 if dist3(rows['E2']['modelPosition'],npcs['exit_E2']['position'])>1e-5:raise ValueError('Unchanged E2 differs')
 updated=[]
 for point in prior['points']:
  row=dict(point);pid=point['id']
  if pid in ('E1','E3'):
   npc=npcs['exit_'+pid];pos=npc['position'];approach=npc['approach']
   if npc['anchor']!=pid or npc['eligibleForRelease'] or not all(npc[x] for x in ('nav','sceneClear','fromSpawnComplete','cameraFrustum','staticSightClear')) or not inside(nav['outer'],pos):raise ValueError('Invalid exit candidate '+pid)
   if dist3(pos,approach)>=3 or distance(pos,rows[pid]['modelPosition'])>3:raise ValueError('Exit stand or displacement differs')
   row.update(modelPosition=pos,standPosition=pos,source='stage-qa-v2',standDistance=0.0,modelDisplacement=distance(pos,old[pid]['originalPlan']))
  row['eligibleForRelease']=False;updated.append(row)
 report=dict(prior);report['points']=updated;report['eligibleForRelease']=False
 report['sourceSHA256']=dict(prior['sourceSHA256']);report['sourceSHA256'].update(r6Input=PIN['prior'],exitStage=PIN['stage'])
 report['manualGates']=list(prior['manualGates'])+['E1/E3 actual NPC prefab, collision and exit interaction coincide on device']
 a.output.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps({'sha256':hashlib.sha256(a.output.read_bytes()).hexdigest(),'points':len(updated),'changed':['E1','E3'],'eligibleForRelease':False}))
if __name__=='__main__':main()
