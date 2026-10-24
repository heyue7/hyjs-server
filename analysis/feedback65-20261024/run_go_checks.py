"""One task-owned managed SQL lease, bounded Go output, unconditional cleanup."""
import fcntl, json, os, subprocess, sys, time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tests'))
from hyjs_managed_mysql import ManagedMySQL

selection = sys.argv[1] if len(sys.argv)>1 else 'TestP7Feedback65|TestP7RushHistorical|TestP7RushSettledReadMemo|TestP7RushPendingProjection63|TestP7EliteManual|TestP7RushContinuation'
out = Path(__file__).parent / (sys.argv[2] if len(sys.argv)>2 else 'go-checks-r1')
pool = ManagedMySQL()
report = {'selection': selection, 'status':'RUNNING'}
began = time.monotonic()
with open('/tmp/hyjs-isolated-mysql.lock','a+') as lock:
    fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
    try:
        conn = pool.allocate('p7_feedback65')
        env = dict(os.environ, **conn.env(), GOMAXPROCS='2', GOMEMLIMIT='768MiB')
        env.update({'HYJS_P7_MYSQL_'+k: env['HYJS_MYSQL_'+k] for k in ['HOST','USER','PASSWORD','DATABASE']})
        command = ['rtk','proxy','go','test','./service','-run',selection,'-count=1','-timeout','240s','-v']
        result = subprocess.run(command, cwd=ROOT/'hyjs-server', env=env, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=270)
        log = pool.redact(result.stdout)
        out.with_suffix('.log').write_text(log)
        report.update(status='PASS' if result.returncode==0 else 'FAIL', exitCode=result.returncode)
        print('\n'.join(line for line in log.splitlines() if line.startswith(('=== RUN','--- PASS','--- FAIL','FAIL','ok\t')) or '.go:' in line)[-18000:])
    finally:
        pool.close()
        report.update(wallSeconds=round(time.monotonic()-began,3), managedSchemas=0, managedAccounts=0)
        out.with_suffix('.json').write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
sys.exit(0 if report['status']=='PASS' else 1)
