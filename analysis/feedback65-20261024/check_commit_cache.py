"""Root reuses its held QA lease, pauses the owned app, runs one separate managed DB test, then resumes it."""
import json,os,signal,subprocess,sys,time
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];OUT=Path(__file__).parent
sys.path.insert(0,str(ROOT/'tests'))
from hyjs_managed_mysql import ManagedMySQL
prefix=sys.argv[2] if len(sys.argv)>2 else 'go-commit-cache-r6'
assert prefix.replace('-','').isalnum()
selection=sys.argv[1] if len(sys.argv)>1 else 'TestP7Feedback65RePrerequisiteReusesAcceptedHistory$|TestP7RushHistoricalCacheBoundsAndDatabaseIsolation$'
owned=json.loads((ROOT/'.local-env/feedback65/processes.json').read_text())
assert owned['database'].startswith('hyjs_test_feedback65_')
wrapper=subprocess.check_output(['rtk','proxy','ps','-p',str(owned['wrapperPid']),'-o','command='],text=True)
app=subprocess.check_output(['rtk','proxy','ps','-p',str(owned['appPid']),'-o','command='],text=True)
assert 'feedback65-20261024/qa_runtime.py' in wrapper and str(ROOT/'build/hyjs-feedback65-current') in app
pool=ManagedMySQL();report={'selection':selection,'status':'RUNNING','leaseOwner':'Root feedback65 existing QA wrapper','qaAppPaused':True};began=time.monotonic()
os.kill(owned['appPid'],signal.SIGSTOP)
try:
 state=subprocess.check_output(['rtk','proxy','ps','-p',str(owned['appPid']),'-o','state='],text=True).strip();assert state.startswith('T')
 conn=pool.allocate('p7_feedback65_final');env=dict(os.environ,**conn.env(),GOMAXPROCS='2',GOMEMLIMIT='768MiB')
 env.update({'HYJS_P7_MYSQL_'+k:env['HYJS_MYSQL_'+k]for k in ['HOST','USER','PASSWORD','DATABASE']})
 r=subprocess.run(['rtk','proxy','go','test','./service','-run',report['selection'],'-count=1','-timeout','240s','-v'],cwd=ROOT/'hyjs-server',env=env,capture_output=True,text=True,timeout=270)
 log=pool.redact(r.stdout+r.stderr);(OUT/(prefix+'.log')).write_text(log);report.update(status='PASS' if r.returncode==0 else 'FAIL',exitCode=r.returncode)
 print('\n'.join(x for x in log.splitlines()if x.startswith(('=== RUN','--- PASS','--- FAIL','FAIL','ok\t'))or '.go:'in x)[-10000:])
finally:
 pool.close();os.kill(owned['appPid'],signal.SIGCONT);report.update(seconds=round(time.monotonic()-began,3),qaAppResumed=True,managedSchemas=0,managedAccounts=0)
 (OUT/(prefix+'.json')).write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report));sys.exit(0 if report['status']=='PASS' else 1)
