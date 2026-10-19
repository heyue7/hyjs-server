#!/usr/bin/env python3
"""Review or atomically replace one legacy rotating occurrence with three exact world subsets."""
import argparse,copy,getpass,http.cookiejar,json,os,urllib.error,urllib.parse,urllib.request

def main():
 parser=argparse.ArgumentParser(description=__doc__)
 parser.add_argument('--base-url',required=True);parser.add_argument('--username',required=True)
 parser.add_argument('--server-id',type=int,default=1);parser.add_argument('--source-occurrence',required=True)
 parser.add_argument('--apply',action='store_true');parser.add_argument('--expected-source-version')
 args=parser.parse_args()
 if args.apply and (not args.expected_source_version or len(args.expected_source_version)!=64):parser.error('--apply requires reviewed --expected-source-version')
 base=args.base_url.rstrip('/')
 if not base.endswith('/admin/api'):base+='/admin/api'
 opener=urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()));csrf=''
 def call(method,path,body=None):
  headers={'Content-Type':'application/json'}
  if csrf and method!='GET':headers['X-CSRF-Token']=csrf
  request=urllib.request.Request(base+path,method=method,headers=headers,data=None if body is None else json.dumps(body).encode())
  try:
   with opener.open(request,timeout=30) as response:result=json.load(response)
  except urllib.error.HTTPError as e:raise RuntimeError(f'{method} {path}: HTTP {e.code}; response and credentials not logged') from None
  if result.get('code',0)!=0:raise RuntimeError(f'{method} {path}: business request rejected; response not logged')
  return result.get('data',result)
 password=os.environ.get('HYJS_ADMIN_PASSWORD') or getpass.getpass('Admin password: ')
 login=call('POST','/auth/login',{'username':args.username,'password':password});csrf=login['csrfToken'];password=None
 try:
  calendar=call('GET',f'/activity-schedules?serverId={args.server_id}')
  source=next((r for r in calendar['items'] if r['id']==args.source_occurrence),None)
  if not source or source['mode']!='king-recruit' or source['form']['options'].get('kingWorld',0)!=0:raise RuntimeError('Exact legacy rotating source occurrence required')
  if args.apply and source['version']!=args.expected_source_version:raise RuntimeError('Reviewed source occurrence changed')
  resources=call('GET','/activity-king-resources')['items'];world_by_id={r['id']:r['space'] for r in resources}
  forms=[]
  for world,label in [(1,'SAO'),(2,'ALO'),(3,'GGO')]:
   form=copy.deepcopy(source['form']);form['options']['kingWorld']=world
   form['name']='王者招募·'+label;form['source']=f'原已发布王者奖池按世界拆分；来源排期{source["id"]}；世界{world}；奖品和权重逐项保持。'
   form['prizes']=[p for p in form['prizes'] if world_by_id.get(p['resourceId'])==world]
   if not form['prizes'] or not any(p['guaranteed'] for p in form['prizes']):raise RuntimeError('Source lacks an original guaranteed candidate for world '+str(world))
   forms.append(form)
  if sum(len(f['prizes']) for f in forms)!=len(source['form']['prizes']):raise RuntimeError('Unknown or cross-world source prize; no changes made')
  plan={'sourceOccurrenceId':source['id'],'expectedSourceVersion':source['version'],'sourceHistoryPreserved':True,'oldOccurrenceEnabledAfterPublish':False,'worlds':[{'world':f['options']['kingWorld'],'name':f['name'],'prizeCount':len(f['prizes']),'prizes':f['prizes']} for f in forms],'originalSharedCountersAndReceiptsUnchanged':True,'startAt':source['startAt'],'endAt':source['endAt'],'status':'READ_ONLY_REVIEW'}
  if not args.apply:print(json.dumps(plan,ensure_ascii=False,indent=2));return
  definitions=call('GET','/activity-templates')['items'];created=[]
  for form in forms:
   existing=[d for d in definitions if d['mode']=='king-recruit' and d['form'].get('source')==form['source']]
   if len(existing)>1:raise RuntimeError('Duplicate staged world templates; review before retry')
   if existing:
    definition=existing[0]
    if not definition['enabled'] or definition['form']['prizes']!=form['prizes'] or definition['form']['options'].get('kingWorld')!=form['options']['kingWorld']:raise RuntimeError('Staged template changed; review before retry')
   else:
    duration=max(1,min(366,(source['endAt']-source['startAt']+86399999)//86400000))
    definition=call('POST','/activity-templates',{'name':form['name'],'form':form,'defaultDays':duration,'claimDays':0,'enabled':True})
   created.append(definition)
  # Refetch the whole calendar after staging; concurrent edits remain version protected.
  calendar=call('GET',f'/activity-schedules?serverId={args.server_id}');items=copy.deepcopy(calendar['items'])
  current=next((r for r in items if r['id']==source['id']),None)
  if not current or current['version']!=args.expected_source_version:raise RuntimeError('Source changed during staging; unscheduled templates retained for review')
  current['enabled']=False
  for definition in created:
   existing=[r for r in items if r['templateId']==definition['id'] and r['startAt']==source['startAt'] and r['endAt']==source['endAt']]
   if len(existing)>1:raise RuntimeError('Duplicate world occurrences; review before retry')
   if existing:existing[0]['enabled']=True;continue
   item=copy.deepcopy(source)
   item.update({'id':'','version':'','consumerId':'','templateId':definition['id'],'templateVersion':definition['version'],'name':definition['name'],'form':definition['form'],'frozen':False,'enabled':True})
   items.append(item)
  body={'version':calendar['version'],'items':items}
  preview=call('POST',f'/activity-schedules/preview?serverId={args.server_id}',body)
  if not preview['valid']:raise RuntimeError('Schedule conflict: '+json.dumps(preview['conflicts'],ensure_ascii=False))
  result=call('PUT',f'/activity-schedules?serverId={args.server_id}',body)
  plan['status']='PUBLISHED';plan['scheduleVersion']=result['version'];plan['worldOccurrences']=[{k:r[k] for k in ['id','name','enabled','consumerId']} for r in result['items'] if r['templateId'] in {d['id'] for d in created}]
  print(json.dumps(plan,ensure_ascii=False,indent=2))
 finally:call('POST','/auth/logout',{})
if __name__=='__main__':main()
