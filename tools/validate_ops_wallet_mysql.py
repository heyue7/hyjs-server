"""Run the approved transaction representatives on one managed allocation at a time."""
import json,os,subprocess,sys,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
sys.path.insert(0,str(ROOT/'tests'))
from hyjs_managed_mysql import ManagedMySQL

def main():
 root=ROOT/'analysis/admin-wallet-polish-20261017';root.mkdir(exist_ok=True)
 candidate=ROOT/'build/candidates/admin-ops-wallet-r2-20261018'
 cases=['TestAdminTypedActivityOriginalCommerceVIPAndCold','TestAdminTypedLoginOriginalConsumerAndTemplateWhitelist','TestAdminGamePlayerRenameKickPreservesOriginalReceipts','TestAdminWalletReasonsRangesAndFrozenCredit','TestAdminWalletLedgerRealWindowPaginationAndIsolation','TestAdminWalletClockRevisionRecoveryAndPermissions']
 pool=ManagedMySQL();results=[];started=time.monotonic()
 try:
  for i,name in enumerate(cases):
   conn=pool.allocate('p1_opswallet'+str(i))
   env=dict(os.environ,GOTOOLCHAIN='go1.27.2',GOMAXPROCS='2',GOMEMLIMIT='768MiB',HYJS_P1_MYSQL_HOST=conn.host,HYJS_P1_MYSQL_USER=conn.user,HYJS_P1_MYSQL_PASSWORD=conn.password,HYJS_P1_MYSQL_DATABASE=conn.database)
   began=time.monotonic();run=subprocess.run(['rtk','proxy','go','test','-json','-count=1','-p=1','-run','^'+name+'$','-timeout','4m','./service'],cwd=candidate/'hyjs-server',env=env,capture_output=True,text=True)
   (root/('mysql-'+str(i)+'-r1.log')).write_text(run.stdout+run.stderr)
   rows=[]
   for line in run.stdout.splitlines():
    try:x=json.loads(line)
    except:continue
    if x.get('Action') in ['pass','fail'] and x.get('Test'):rows.append({k:x[k]for k in ['Action','Test','Elapsed']if k in x})
   item={'selector':name,'exitCode':run.returncode,'elapsedSeconds':round(time.monotonic()-began,3),'database':conn.database,'results':rows}
   results.append(item);print(json.dumps(item),flush=True);pool.release(conn.database)
   if run.returncode:break
 finally:
  pool.close()
  report={'status':'PASS' if len(results)==len(cases) and all(x['exitCode']==0 for x in results)else'FAILED','results':results,'ownedDatabasesAndUsersReleased':not bool(pool.connections),'elapsedSeconds':round(time.monotonic()-started,3)}
  (root/'mysql-validation-r1.json').write_text(json.dumps(report,indent=2));print(json.dumps({k:report[k]for k in ['status','ownedDatabasesAndUsersReleased','elapsedSeconds']}),flush=True)
 if report['status']!='PASS':raise SystemExit(1)
if __name__=='__main__':main()
