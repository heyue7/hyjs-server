"""Install the fixed formal candidate without QA credentials, then restore and verify the phone lease."""
from pathlib import Path
import gzip,hashlib,json,os,re,subprocess,sys,time,tarfile,io
ROOT=Path(__file__).resolve().parents[2];OUT=Path(__file__).parent;PRIVATE=ROOT/'.local-env/feedback65/phone'
sys.path.insert(0,str(ROOT/'tools'))
from device_demo import Device,ADB
d=Device('10AF7Q0KSJ003GK');lease=json.loads((OUT/'phone-lease.json').read_text());mode=sys.argv[1]
assert mode in ('install','restore')
def into(path,data):
 r=subprocess.run(['rtk','proxy',str(ADB),'-s',d.serial,'exec-in','run-as',d.package,'tee',path],input=data,stdout=subprocess.DEVNULL,stderr=subprocess.PIPE,timeout=30)
 assert r.returncode==0,'Private owned file write failed'
def clear_prefs():
 for name in d.text('shell','run-as',d.package,'ls','shared_prefs').splitlines():
  assert re.fullmatch(r'[A-Za-z0-9_.-]+',name)
  d.run('shell','run-as',d.package,'rm','shared_prefs/'+name)
if mode=='install':
 d.run('shell','am','force-stop',d.package)
 installed=d.text('shell','pm','path',d.package).split(':',1)[1];assert d.text('shell','sha256sum',installed,timeout=60).split()[0]=='d995f7bc774381cd24ca85f3ab2b8a6adea5c07a7b8d9610f6fb3598dee8b3ca'
 clear_prefs();into('shared_prefs/'+d.package+'.v2.playerprefs.xml',(PRIVATE/'tutorial-prefs.xml').read_bytes())
 into('files/native-runner-current.dll',(PRIVATE/'original-caller.dll').read_bytes())
 assert d.text('shell','run-as',d.package,'sha256sum','files/native-runner-current.dll').split()[0]==lease['originalCallerSHA256']
 apk=ROOT/'build/hyjs-feedback65-formal-r4.apk';assert hashlib.file_digest(apk.open('rb'),'sha256').hexdigest()=='f26942c80a6b39d2b83eeed63a0775dff7e84147db815f553256e09c06026ba7'
 report=d.install(apk);time.sleep(8);report['capture']=d.capture(OUT/'phone-formal65-startup.png');report['productionGameLoginSubmitted']=False
 d.run('shell','am','force-stop',d.package);(OUT/'formal-phone-installed.json').write_text(json.dumps(report,indent=2)+'\n');print(report)
else:
 d.run('shell','am','force-stop',d.package)
 raw=(PRIVATE/'shared-prefs.tar').read_bytes();assert hashlib.sha256(raw).hexdigest()==lease['privateBackupSHA256']
 with tarfile.open(fileobj=io.BytesIO(raw))as t:
  assert all(not Path(x.name).is_absolute() and '..'not in Path(x.name).parts for x in t.getmembers())
 clear_prefs()
 r=subprocess.run(['rtk','proxy',str(ADB),'-s',d.serial,'exec-in','run-as',d.package,'tar','-xf','-','-C','shared_prefs'],input=raw,capture_output=True,timeout=30);assert r.returncode==0,'Original private preference restore failed'
 names=d.text('shell','run-as',d.package,'ls','shared_prefs').splitlines();actual={n:d.text('shell','run-as',d.package,'sha256sum','shared_prefs/'+n).split()[0]for n in names};assert actual==lease['preferenceHashes']
 into('files/native-runner-current.dll',(PRIVATE/'original-caller.dll').read_bytes());assert d.text('shell','run-as',d.package,'sha256sum','files/native-runner-current.dll').split()[0]==lease['originalCallerSHA256']
 observed=d.run('exec-out','run-as',d.package,'cat','files/hyjs65-qa/observations.jsonl');gzip.compress(observed)
 (PRIVATE/'observations.jsonl.gz').write_bytes(gzip.compress(observed));d.run('shell','run-as',d.package,'rm','-rf','files/hyjs65-qa');d.run('shell','run-as',d.package,'rm','-f','files/native-runner-original65.dll')
 def reverses():return {a[1]:a[2]for line in d.text('reverse','--list').splitlines()if len(a:=line.split())==3}
 current=reverses()
 for port in [28979,28980]:
  endpoint='tcp:'+str(port)
  if endpoint in current:
   assert current[endpoint]==endpoint;d.run('reverse','--remove',endpoint)
 for line in lease['originalReverse'].splitlines():
  a=line.split()
  if len(a)!=3:continue
  source,target=a[1:];current=reverses()
  if source not in current:d.run('reverse','--no-rebind',source,target)
  else:assert current[source]==target
 for namespace,name in [('global','stay_on_while_plugged_in'),('system','screen_off_timeout')]:
  value=lease['display'][name];assert value=='null'or value.isdigit()
  if value=='null':d.run('shell','settings','delete',namespace,name)
  else:d.run('shell','settings','put',namespace,name,value)
  assert d.text('shell','settings','get',namespace,name)==value
 d.run('shell','input','keyevent','KEYCODE_HOME');report={'originalPreferenceHashesRestored':len(actual),'originalCallerSHA256':lease['originalCallerSHA256'],'QAProbeRemoved':True,'QAPortsRemoved':True,'originalReverseRestored':True,'displayRestored':True,'gameForceStopped':True,'productionGameLoginSubmitted':False,'observationSHA256':hashlib.sha256(observed).hexdigest()}
 (OUT/'phone-restored.json').write_text(json.dumps(report,indent=2)+'\n');(PRIVATE/'finish').touch();print(report)
