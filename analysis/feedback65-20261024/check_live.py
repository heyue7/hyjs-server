"""Read-only live release verification; never authenticates a game player or grants resources."""
import hashlib,json,shlex,subprocess,sys,urllib.request,re
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT/'deploy'))
from settings import load
s=load('/Users/lirui/LR/hyjs/deploy/.env');phase=sys.argv[1];assert phase in ('before','after')
code="""from pathlib import Path
import json,hashlib,subprocess,os,shutil
root=Path(ROOTVALUE)
def run(*a,input=None):return subprocess.run(a,input=input,capture_output=True,check=True).stdout
def sha(p):
 h=hashlib.sha256()
 with p.open('rb') as f:
  for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
 return h.hexdigest()
m=json.loads((root/'deployed.json').read_text());g=json.loads(run('docker','inspect','hyjs-game'))[0];d=json.loads(run('docker','inspect','hyjs-mysql'))[0]
pid=g['State']['Pid'];h=sha(Path('/proc/'+str(pid)+'/exe'))
sql="SELECT enabled,archived,COUNT(*) FROM admin_activity_occurrences GROUP BY enabled,archived; SELECT id,SHA2(COALESCE(player,''),256),SHA2(COALESCE(game,''),256) FROM profiles WHERE id IN (1,24) ORDER BY id; SELECT kind,status,COUNT(*) FROM p7_rush_fights WHERE status<>'settled' GROUP BY kind,status; SELECT committed,COUNT(*) FROM p12_wanted_fights GROUP BY committed;"
rows=run('docker','exec','-i','hyjs-mysql','sh','-c','MYSQL_PWD="$MYSQL_PASSWORD" exec mysql -uhyjs --batch --raw --skip-column-names hyjs',input=sql.encode()).decode().strip().splitlines()
b=json.loads((root/'backup-latest.json').read_text());bp=root/'backups'/b['file'];bh=sha(bp);assert bh==b['sha256']
print(json.dumps(dict(revision=m['revision'],expectedExecutableSHA256=m['files']['app/hyjs-server'],runningExecutableSHA256=h,runningBinaryMatches=h==m['files']['app/hyjs-server'],appState={k:g['State'].get(k)for k in ['Status','OOMKilled']},appRestartCount=g['RestartCount'],mysqlState={k:d['State'].get(k)for k in ['Status','OOMKilled','Health']},mysqlRestartCount=d['RestartCount'],sqlMetadataAndProfileDigests=rows,backup={'file':bp.name,'sha256':bh,'bytes':bp.stat().st_size},freeBytes=shutil.disk_usage(root).free)))
""".replace('ROOTVALUE',repr(s['HYJS_DEPLOY_ROOT']))
r=subprocess.run(['rtk','proxy','ssh','-o','BatchMode=yes','-o','ConnectTimeout=15',s['HYJS_DEPLOY_SSH'],'sudo -n python3 -c '+shlex.quote(code)],capture_output=True,text=True,timeout=90)
assert r.returncode==0,r.stderr[-1000:];report=json.loads(r.stdout)
# Keep only compact health state; MySQL health output can contain environment diagnostics.
report['mysqlState']['Health']={'Status':report['mysqlState']['Health']['Status']}
checks={}
for name,url in [('health',s['HYJS_API_ORIGIN']+'/health'),('admin',s['HYJS_ADMIN_ORIGIN']+'/admin/')]:
 with urllib.request.urlopen(url,timeout=15)as z:body=z.read();checks[name]=z.status
 if name=='admin':
  for path in re.findall(r'(?:src|href)=["\']([^"\']+\.(?:js|css))["\']',body.decode()):
   with urllib.request.urlopen(s['HYJS_ADMIN_ORIGIN']+path,timeout=15)as q:checks[path]=q.status
req=urllib.request.Request(s['HYJS_API_ORIGIN']+'/sao/',method='OPTIONS',headers={'Origin':s['HYJS_ADMIN_ORIGIN'],'Access-Control-Request-Method':'POST'})
with urllib.request.urlopen(req,timeout=15)as z:checks['corsStatus']=z.status;checks['exactCorsOrigin']=z.headers.get('Access-Control-Allow-Origin')==s['HYJS_ADMIN_ORIGIN']
report['publicHTTP']=checks;report['productionGameLoginsOrRewardsIssued']=False
(ROOT/'analysis/feedback65-20261024'/('live-'+phase+'.json')).write_text(json.dumps(report,indent=2)+'\n')
print(json.dumps(report))
