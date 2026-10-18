"""Build two immutable real engine closures before holding the isolated MySQL lifecycle lock."""
import argparse,hashlib,json,shutil,subprocess,tempfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
def run(args,cwd):
 r=subprocess.run(['rtk','proxy',*args],cwd=cwd,capture_output=True,text=True,timeout=300)
 if r.returncode:raise RuntimeError((r.stdout+r.stderr)[-7000:])
def prepare(legacy,current):
 directory=Path(tempfile.mkdtemp(prefix='hyjs-p14-upgrade-prebuilt-'))
 try:
  for role,base in [('legacy',legacy),('current',current)]:
   source=directory/role
   shutil.copytree(base/'hyjs-server',source/'hyjs-server')
   shutil.copytree(base/'analysis/tables',source/'analysis/tables')
   shutil.copyfile(ROOT/'hyjs-server/service/p14EngineUpgrade_test.go',source/'hyjs-server/service/p14EngineUpgrade_test.go')
   run(['go','test','-race','-c','-o',str(directory/(role+'-race.test')),'./service'],source/'hyjs-server')
  path=directory/'current/hyjs-server/main.go';body=path.read_text()
  if '"strings"' not in body:body=body.replace('"hyjs-server/config"','"hyjs-server/config"\n"strings"',1)
  assert body.count('defer cancel()\n')==1
  body=body.replace('defer cancel()\n','defer cancel()\nif !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_upgrade_") { return errors.New("P14 upgrade isolated schema required") }\n',1)
  path.write_text(body);run(['gofmt','-w',str(path)],ROOT)
  run(['go','build','-o',str(directory/'current-native'),'.'],directory/'current/hyjs-server')
  files={str(p.relative_to(directory)):hashlib.sha256(p.read_bytes()).hexdigest() for role in ['legacy','current'] for folder in ['hyjs-server','analysis/tables'] for p in sorted((directory/role/folder).rglob('*')) if p.is_file()}
  meta={'legacyBaseline':str(legacy),'currentBaseline':str(current),'files':files,'binaries':{name:hashlib.sha256((directory/name).read_bytes()).hexdigest() for name in ['legacy-race.test','current-race.test','current-native']},'scope':'actual old/new complete P6 source, unchanged engine constants; only new owned upgrade tests and disposable native isolated-schema guard'}
  (directory/'manifest.json').write_text(json.dumps(meta,indent=2)+'\n');return directory
 except BaseException:shutil.rmtree(directory);raise
if __name__=='__main__':
 parser=argparse.ArgumentParser();parser.add_argument('--legacy',type=Path,required=True);parser.add_argument('--current',type=Path,required=True);args=parser.parse_args();print(prepare(args.legacy,args.current))
