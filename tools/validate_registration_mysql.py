"""Check account transactions on managed project-only MySQL allocations."""
import argparse,json,os,subprocess,sys,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests'))
from hyjs_managed_mysql import ManagedMySQL

def main():
 parser=argparse.ArgumentParser();parser.add_argument('--candidate',type=Path,required=True);parser.add_argument('--attempt',type=int,required=True);args=parser.parse_args()
 root=ROOT/'analysis/admin-usability-20261018';root.mkdir(exist_ok=True)
 record=root/('mysql-registration-r'+str(args.attempt)+'.json')
 if record.exists():raise ValueError('Keep prior evidence immutable')
 candidate=args.candidate.resolve()
 if candidate.parent!=ROOT/'build/candidates' or not (candidate/'manifest.json').is_file():raise ValueError('Use pinned candidate')
 cases=['TestGameAccountRegisterStrictDuplicateConcurrentAndOriginalLogin','TestAdminGamePlayerCreateOriginalStarterRBACAuditAndDuplicate']
 pool=ManagedMySQL();results=[];started=time.monotonic()
 try:
  for i,name in enumerate(cases):
   conn=pool.allocate('p1_register'+str(i))
   env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMAXPROCS='2',GOMEMLIMIT='768MiB',HYJS_P1_MYSQL_HOST=conn.host,HYJS_P1_MYSQL_USER=conn.user,HYJS_P1_MYSQL_PASSWORD=conn.password,HYJS_P1_MYSQL_DATABASE=conn.database)
   began=time.monotonic();run=subprocess.run(['rtk','proxy','go','test','-json','-count=1','-p=1','-run','^'+name+'$','-timeout','4m','./service'],cwd=candidate/'hyjs-server',env=env,capture_output=True,text=True)
   rows=[];failures=[]
   for line in run.stdout.splitlines():
    try:x=json.loads(line)
    except:continue
    if x.get('Action') in ['pass','fail'] and x.get('Test'):rows.append({k:x[k]for k in ['Action','Test','Elapsed']if k in x})
    if x.get('Test') and x.get('Output'):failures.append(x['Output'])
   item={'selector':name,'exitCode':run.returncode,'elapsedSeconds':round(time.monotonic()-began,3),'database':conn.database,'results':rows}
   if run.returncode:item['failureTail']=failures[-10:]
   results.append(item);print(json.dumps(item),flush=True);pool.release(conn.database)
   if run.returncode:break
 finally:
  pool.close()
  report={'candidate':str(candidate.relative_to(ROOT)),'status':'PASS' if len(results)==len(cases) and all(x['exitCode']==0 for x in results)else'FAILED','results':results,'ownedDatabasesAndUsersReleased':not bool(pool.connections),'elapsedSeconds':round(time.monotonic()-started,3)}
  record.write_text(json.dumps(report,indent=2));print(json.dumps({k:report[k]for k in ['status','ownedDatabasesAndUsersReleased','elapsedSeconds']}),flush=True)
 if report['status']!='PASS':raise SystemExit(1)
if __name__=='__main__':main()
