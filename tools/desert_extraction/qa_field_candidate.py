"""Build and, only after explicit review, submit a local-only desert QA configuration.

The draft command never saves a template or deploys. An incomplete draft has
empty entryPoints, ready=false, and cannot be submitted. This tool is restricted
to the dedicated loopback QA process, fixture identity and optimistic versions.
It must never be used for production.
"""
import argparse,copy,hashlib,http.cookiejar,json,math,os,subprocess,time,urllib.request
from collections import Counter
from pathlib import Path

ROOT=Path(__file__).resolve().parents[2]
PRIVATE=Path('/private/tmp/hyjs-desert-v4-qa')
BASE='http://127.0.0.1:18861/admin/api'
DATABASE='hyjs_test_p1_desert_v4_qa_28b'
POINT_SHA='3257cce0d7fb048299dee9cf894d67e9619eebaf6386655faf2294ffb20e60b5'
STAGE_SHA='8eca4d81f62abbccf121fb13efbc33c80338208ab5e9d66e5922d253b5a46ca8'
ROLE3_SHA='6c1d9d0f1687fc1df913ef66a63bc780a1c314a186cf635bb60be8f4002f39e8'
ENTRY_SHA='c3e92afc8a35860b1d3264450db5fcea1b9e65a40a29e6b37f5c1d8f1c830ac8'
POINT_AUDIT_SHA='61ec8c58480d8e4ee3c53f7ccdf13d0cb09a96f86407fc81224cfd0ea75ca8f2'
BUILD=2026102804

def sha(raw):return hashlib.sha256(raw).hexdigest()
def load(path,pin=None):
 raw=Path(path).read_bytes()
 if pin is not None and sha(raw)!=pin:raise ValueError('Source SHA mismatch')
 return json.loads(raw),sha(raw)
def point(p):
 if not isinstance(p,dict) or set(p)!={'x','y','z'}:raise ValueError('Invalid position')
 if any(isinstance(p[k],bool) or not isinstance(p[k],(int,float)) or not math.isfinite(p[k]) or abs(p[k])>100000 for k in p):raise ValueError('Invalid position value')
 return {k:float(p[k]) for k in ('x','y','z')}
def d3(a,b):return math.sqrt(sum((a[k]-b[k])**2 for k in ('x','y','z')))
def inside(p,box):return box['minX']<=p['x']<=box['maxX'] and box['minZ']<=p['z']<=box['maxZ']
def isolated_identity():
 env,_=load(PRIVATE/'qa-env.json'); fixture,_=load(PRIVATE/'fixture.json')
 if env.get('HYJS_MYSQL_DATABASE')!=DATABASE or env.get('HYJS_GAME_ACCOUNTS_DATABASE') not in ('',None) or env.get('HYJS_MYSQL_HOST')!='127.0.0.1:3306':raise ValueError('Unexpected QA database')
 if fixture.get('schema')!=DATABASE or fixture.get('http')!='http://127.0.0.1:18861' or fixture.get('tcpHost')!='127.0.0.1' or fixture.get('tcpPort')!=18862:raise ValueError('Unexpected QA fixture')
 # The existing QA flag is accepted by the server only with loopback and a
 # test database. Inspect the listener and process without logging arguments.
 result=subprocess.run(['lsof','-nP','-iTCP:18861','-sTCP:LISTEN','-F','pn'],capture_output=True,text=True,check=True)
 names=[x[1:] for x in result.stdout.splitlines() if x.startswith('n')]
 pids=[x[1:] for x in result.stdout.splitlines() if x.startswith('p')]
 if names!=['127.0.0.1:18861'] or len(pids)!=1 or not pids[0].isdigit():raise ValueError('Unexpected QA listener')
 cmd=subprocess.run(['ps','-p',pids[0],'-o','command='],capture_output=True,text=True,check=True).stdout
 if '--enable-ggo-integration' not in cmd or '/hyjs-desert-v4-qa/' not in cmd:raise ValueError('QA process identity mismatch')
 return fixture

def build_draft(base,published,selected,stage):
 if base.get('schema')!='hyjs-desert-v4-local-qa-field-base-v1' or not base.get('qaOnly') or base.get('serverId')!=1:raise ValueError('Invalid QA baseline')
 template,deployment=base['template'],base['deployment'];cfg=copy.deepcopy(template['configuration'])
 if template['id']!='original-field_003' or cfg['scene']!='field_003' or deployment['scene']!='field_003' or deployment['templateId']!=template['id'] or len(template['version'])!=64 or len(deployment['version'])!=64:raise ValueError('QA field identity changed')
 # The supplied running snapshot and current template may differ only by the
 # separately reviewed boss HP adjustment. Preserve every other QA setting.
 old=copy.deepcopy(published);cur=copy.deepcopy(cfg)
 old['ggo']['runtime']['boss']['hp']=cur['ggo']['runtime']['boss']['hp']
 if old!=cur:raise ValueError('Template differs from deployed settings beyond boss HP')
 if selected.get('schema')!='hyjs-desert-v4-selected-audit-input-v1' or len(selected['points'])!=59 or selected['eligibleForRelease']:raise ValueError('Unexpected point candidates')
 if stage.get('schema')!='hyjs-desert-v4-stage-qa-v2' or stage['eligibleForRelease'] or stage['unityVersion']!='5.3.3p3' or stage['navVertices']!=4947 or stage['navTriangles']!=2337:raise ValueError('Unexpected stage audit')
 source={p['id']:p for p in selected['points']}; saved={p['id']:p for p in cfg['ggo']['runtime']['points']}
 if len(source)!=59 or len(saved)!=58 or set(source)-{'S'}!=set(saved):raise ValueError('Point identities differ')
 if Counter(p['kind'] for p in saved.values())!={'monster':24,'mine':18,'fish':12,'boss':1,'exit':3}:raise ValueError('QA point count differs')
 patrol={p['id']:p for p in stage['patrols']}
 if set(patrol)!={'M01','M02','M03','M04'}:raise ValueError('Bridge patrol set differs')
 for key,row in saved.items():
  original=source[key]; expected='monster' if original['kind']=='mob' else original['kind']
  if original['eligibleForRelease'] or row['kind']!=expected:raise ValueError('Point kind differs: '+key)
  row['position']=point(original['modelPosition']);row['standPosition']=point(original['standPosition'])
  if key in patrol:
   p=patrol[key];way=[point(v) for v in p['waypoints']]
   if p['eligibleForRelease'] or len(way)!=2 or not all(p[x] for x in ('endpointsOnNav','pathComplete','sceneSweepClear')) or abs(d3(*way)-3)>0.002 or abs(p['speed']-3.2)>0.001:raise ValueError('Bridge patrol audit differs: '+key)
   row['position']=way[0];row['standPosition']=way[0];row['patrol']={'waypoints':way,'speed':3.2}
  elif row.get('patrol'):
   raise ValueError('Unexpected nonbridge patrol')
 g=cfg['ggo'];runtime=g['runtime'];safe=point(source['S']['modelPosition']);area={'minX':-11.0,'maxX':3.0,'minZ':-10.0,'maxZ':1.0}
 if not inside(safe,area) or d3(safe,point(stage['anchor']))>1e-5:raise ValueError('Safe area anchor differs')
 stage_npcs={p['id']:p for p in stage['npcs']};bindings={'shino':'guide','kohl':'trade','dane':'exit_E1','kuroe':'exit_E2','serra':'exit_E3'}
 npcs={n['id']:n for n in g['npcs']}
 if set(npcs)!=set(bindings) or len(stage_npcs)!=5:raise ValueError('NPC identities differ')
 for npc_id,stage_id in bindings.items():
  src=stage_npcs[stage_id]
  if src['eligibleForRelease'] or not all(src[x] for x in ('nav','sceneClear','fromSpawnComplete','cameraFrustum','staticSightClear')):raise ValueError('NPC audit failed: '+npc_id)
  p=point(src['position']);npcs[npc_id]['position']=p
  if npc_id in ('shino','kohl') and not inside(p,area):raise ValueError('Safe NPC outside area')
  if stage_id.startswith('exit_') and d3(p,saved[stage_id[5:]]['position'])>1e-5:raise ValueError('NPC and extraction point differ')
 runtime['safeArea']=area;runtime['safeReturn']=safe;runtime['entryPoints']=[]
 runtime['boss']['hp']=134400000;runtime['minClientBuild']=BUILD
 if runtime['resource']['mineCount']!=18 or runtime['resource']['fishCount']!=12:raise ValueError('Resource counts differ')
 return {'schema':'hyjs-desert-v4-qa-configuration-candidate-v1','qaOnly':True,'neverPromote':True,'serverId':1,
         'templateId':template['id'],'templateName':template['name'],'templateVersion':template['version'],'deploymentVersion':deployment['version'],
         'templateChangedAtRead':deployment['templateChanged'],'deploymentEnabledAtRead':deployment['enabled'],
         'configuration':cfg,'ready':False,'pendingReasons':['dangerous_entry_points_need_unity_qa_audit','role3_build_2026102804_not_verified','r7_automatic_point_audit_required'],
         'limitations':['candidate_world_coordinates_not_approved_for_release','native_npc_and_patrol_physics_need_device_validation'],
         'sourcePins':{'pointSelection':POINT_SHA,'stageAudit':STAGE_SHA},'serverWriteExecuted':False}

def hdist(a,b):return math.hypot(a['x']-b['x'],a['z']-b['z'])
def hsegment(p,a,b):
 dx=b['x']-a['x'];dz=b['z']-a['z'];length=dx*dx+dz*dz
 if not length:return hdist(p,a)
 t=max(0.0,min(1.0,((p['x']-a['x'])*dx+(p['z']-a['z'])*dz)/length))
 return math.hypot(p['x']-a['x']-t*dx,p['z']-a['z']-t*dz)

def add_entry_proof(c,selected,stage,report):
 if report.get('schema')!='hyjs-desert-v4-entry-qa-v1' or report.get('unityVersion')!='5.3.3p3' or report.get('scene')!=stage['scene'] or report.get('navVertices')!=4947 or report.get('navTriangles')!=2337 or report.get('eligibleForRelease') is not False:raise ValueError('Unexpected entry-point audit')
 rows=report.get('entryPoints')
 if not isinstance(rows,list) or len(rows)!=4 or {r.get('id') for r in rows}!={'entry_1','entry_2','entry_3','entry_4'}:raise ValueError('Unexpected entry-point identities')
 origin=point(report['safeReturn']);r=c['configuration']['ggo']['runtime'];area=r['safeArea']
 if not inside(origin,area) or d3(origin,point(stage['originalSpawn']))>0.02:raise ValueError('Audits have different safe path origins')
 points=selected['points'];entries=[];configured=r['points'];npcs=c['configuration']['ggo']['npcs']
 for row in sorted(rows,key=lambda v:v['id']):
  if row.get('eligibleForRelease') is not False or not all(row.get(k) is True for k in ('outsideSafe','outsideBridge','nav','sceneClear','pathComplete')):raise ValueError('Entry-point audit did not pass')
  pos=point(row['position'])
  if inside(pos,area) or any(hdist(pos,old)<12.0-0.02 for old in entries):raise ValueError('Entry-point safe area or spacing differs')
  distances=[(hdist(pos,point(q['modelPosition'])),q) for q in points]
  nearest,q=min(distances,key=lambda v:v[0])
  if nearest<4.0-0.02 or abs(nearest-float(row['nearestObjectDistance']))>0.03 or row.get('nearestObject')!=q['id']:raise ValueError('Entry-point nearest object differs')
  if any(q['kind']=='boss' and d<6.0-0.02 for d,q in distances):raise ValueError('Entry-point boss distance differs')
  for item in configured:
   p=point(item['position']);minimum=6.0 if item['kind']=='boss' else 4.0
   if hdist(pos,p)<minimum-0.02:raise ValueError('Entry point too near configured object')
   patrol=item.get('patrol')
   if patrol and any(hsegment(pos,point(a),point(b))<4.0-0.02 for a,b in zip(patrol['waypoints'],patrol['waypoints'][1:])):raise ValueError('Entry point too near bridge patrol')
  if any(hdist(pos,point(npc['position']))<4.0-0.02 for npc in npcs):raise ValueError('Entry point too near configured NPC')
  entries.append(pos)
 r['safeReturn']=origin;r['entryPoints']=entries;c['sourcePins']['entryAudit']=ENTRY_SHA
 c['pendingReasons'].remove('dangerous_entry_points_need_unity_qa_audit')
 c['limitations'].append('entry_dynamic_actor_motion_and_original_device_need_validation')

def add_point_audit(c,selected,stage,audit):
 if audit.get('schema')!='hyjs-desert-v4-selected-audit-report-v1' or audit.get('unityVersion')!='5.3.3p3' or audit.get('scene')!=stage['scene'] or audit.get('inputSchema')!=selected['schema'] or audit.get('bundleSha256')!='2d696b559ae5990b1be9978968e11fbb181ccfe1004a4cdd21cd8d1b435d05d1' or audit.get('allEligibleForRelease') is not False or audit.get('navVertices')!=4947 or audit.get('navTriangles')!=2337 or audit.get('pointCount')!=59 or audit.get('automaticPassCount')!=59 or d3(point(audit['spawn']),point(stage['originalSpawn']))>0.02:raise ValueError('Unexpected 59-point audit')
 rows=audit.get('points')
 if not isinstance(rows,list) or len(rows)!=59:raise ValueError('Unexpected audited points')
 source={p['id']:p for p in selected['points']};checked={p['id']:p for p in rows}
 if len(checked)!=59 or set(source)!=set(checked):raise ValueError('Audited point identities differ')
 for key,p in checked.items():
  src=source[key]
  if p.get('kind')!=src['kind'] or p.get('automatedChecksPass') is not True or p.get('eligibleForRelease') is not False or d3(point(p['modelPosition']),point(src['modelPosition']))>0.0001 or d3(point(p['standPosition']),point(src['standPosition']))>0.0001:raise ValueError('Audited point differs: '+key)
 c['sourcePins']['selectedPointAudit']=POINT_AUDIT_SHA;c['pendingReasons'].remove('r7_automatic_point_audit_required')
 c['limitations'].append('automatic_point_checks_do_not_confirm_fish_visibility_or_native_device')

def add_role3_proof(c,proof):
 if proof.get('environment')!='isolated local QA' or proof.get('database')!=DATABASE or proof.get('serverId')!=1 or proof.get('roleId')!=3 or proof.get('buildCode')!=BUILD or proof.get('sessionMatches') is not True or proof.get('socketHost')!='127.0.0.1' or proof.get('socketPort')!=18862:raise ValueError('Role 3 QA build evidence differs')
 c['sourcePins']['role3BuildProof']=ROLE3_SHA;c['pendingReasons'].remove('role3_build_2026102804_not_verified')
 c['limitations'].append('role3_build_binding_must_still_be_current_at_qa_publish')

def candidate_from_files(base_path,published_path,points_path,stage_path,entry_path=None,role3_path=None,point_audit_path=None):
 base,b=load(base_path);published,pub=load(published_path);selected,_=load(points_path,POINT_SHA);stage,_=load(stage_path,STAGE_SHA)
 c=build_draft(base,published,selected,stage);c['sourcePins'].update(templateRead=b,publishedRead=pub)
 if entry_path:
  entry,_=load(entry_path,ENTRY_SHA);add_entry_proof(c,selected,stage,entry)
 if role3_path:
  proof,_=load(role3_path,ROLE3_SHA);add_role3_proof(c,proof)
 if point_audit_path:
  audit,_=load(point_audit_path,POINT_AUDIT_SHA);add_point_audit(c,selected,stage,audit)
 c['ready']=not c['pendingReasons']
 return c

class Admin:
 def __init__(self):
  self.jar=http.cookiejar.CookieJar();self.opener=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPCookieProcessor(self.jar));self.csrf=''
 def call(self,method,path,data=None):
  if not path.startswith('/') or '://' in path:raise ValueError('Unexpected admin path')
  body=None if data is None else json.dumps(data,separators=(',',':'),ensure_ascii=False).encode()
  headers={'Content-Type':'application/json'}
  if self.csrf:headers['X-CSRF-Token']=self.csrf
  request=urllib.request.Request(BASE+path,data=body,headers=headers,method=method)
  with self.opener.open(request,timeout=15) as reply:r=json.load(reply)
  if r.get('code')!=0:raise RuntimeError('QA admin operation rejected')
  return r['data']
 def login(self):
  credentials,_=load(PRIVATE/'admin.json');self.csrf=self.call('POST','/auth/login',credentials)['csrfToken']
 def logout(self):
  if self.csrf:
   try:self.call('POST','/auth/logout',{})
   finally:self.csrf=''

def validate_submit(c,base,published,points,stage,entry,role3,point_audit):
 if c.get('schema')!='hyjs-desert-v4-qa-configuration-candidate-v1' or c.get('qaOnly') is not True or c.get('neverPromote') is not True or c.get('ready') is not True or c.get('pendingReasons'):
  raise ValueError('Incomplete QA candidate cannot be submitted')
 expected=candidate_from_files(base,published,points,stage,entry,role3,point_audit)
 if expected!=c:raise ValueError('Reviewed candidate differs from pinned QA evidence')

def submit(path,reviewed_sha,base,published,points,stage,entry,role3,point_audit):
 raw=Path(path).read_bytes()
 if len(reviewed_sha)!=64 or sha(raw)!=reviewed_sha:raise ValueError('Reviewed candidate SHA differs')
 c=json.loads(raw);validate_submit(c,base,published,points,stage,entry,role3,point_audit);fixture=isolated_identity()
 if len([p for p in fixture['players'] if p.get('playerId')==3])!=1:raise ValueError('Role 3 QA fixture missing')
 # The caller must review the candidate and freshly confirm the role-3 build
 # binding. No production or non-loopback target is accepted.
 api=Admin();saved=None
 try:
  api.login();cat=api.call('GET','/fields/templates');deps=api.call('GET','/fields/deployments?serverId=1')
  t=[x for x in cat['items'] if x['id']==c['templateId']];d=[x for x in deps['items'] if x['scene']=='field_003']
  if len(t)!=1 or len(d)!=1 or t[0]['version']!=c['templateVersion'] or d[0]['version']!=c['deploymentVersion'] or d[0]['templateId']!=c['templateId']:raise ValueError('QA versions changed; review a new candidate')
  saved=api.call('POST','/fields/templates',{'id':c['templateId'],'name':c['templateName'],'version':c['templateVersion'],'configuration':c['configuration']})
  result=api.call('PUT','/fields/deployments',{'serverId':1,'scene':'field_003','templateId':saved['id'],'templateVersion':saved['version'],'version':c['deploymentVersion'],'enabled':True})
  print(json.dumps({'qaOnly':True,'templateId':saved['id'],'savedVersion':saved['version'],'serverId':result['serverId'],'published':True}))
 except Exception:
  if saved is not None:print(json.dumps({'qaOnly':True,'templateSaved':True,'savedVersion':saved['version'],'deploymentConfirmed':False}))
  raise
 finally:api.logout()

def main():
 p=argparse.ArgumentParser(description=__doc__);sub=p.add_subparsers(dest='command',required=True)
 d=sub.add_parser('draft');q=sub.add_parser('submit')
 for parser in (d,q):
  parser.add_argument('--base',type=Path,required=True);parser.add_argument('--published',type=Path,required=True);parser.add_argument('--points',type=Path,required=True);parser.add_argument('--stage',type=Path,required=True);parser.add_argument('--entry-audit',type=Path,required=parser is q);parser.add_argument('--role3-build-proof',type=Path,required=parser is q);parser.add_argument('--point-audit',type=Path,required=parser is q)
 d.add_argument('--output',type=Path,required=True)
 q.add_argument('--candidate',type=Path,required=True);q.add_argument('--reviewed-sha',required=True);q.add_argument('--confirm-local-qa',action='store_true')
 a=p.parse_args()
 if a.command=='draft':
  if a.output.exists() or not a.output.resolve().is_relative_to(ROOT/'analysis/desert-extraction-v4-client'):raise ValueError('Fresh owned draft output required')
  c=candidate_from_files(a.base,a.published,a.points,a.stage,a.entry_audit,a.role3_build_proof,a.point_audit)
  a.output.write_text(json.dumps(c,ensure_ascii=False,indent=2)+'\n')
  print(json.dumps({'candidate':str(a.output.resolve()),'sha256':sha(a.output.read_bytes()),'ready':c['ready'],'pendingReasons':c['pendingReasons'],'serverWriteExecuted':False}))
 else:
  if not a.confirm_local_qa:raise ValueError('Explicit local QA flag required')
  submit(a.candidate,a.reviewed_sha,a.base,a.published,a.points,a.stage,a.entry_audit,a.role3_build_proof,a.point_audit)
if __name__=='__main__':main()
