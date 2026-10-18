"""Precompile exact domain/shared fixed source outside SQL lock, native guard admits only own fixture schemas."""
import argparse,hashlib,json,shutil,subprocess,tempfile
from pathlib import Path
def prepare(source):
 assert source.name.startswith('hyjs-p14-season-qualification-')
 out=Path(tempfile.mkdtemp(prefix='hyjs-p14-qualification-prebuilt-'))
 try:
  shutil.copytree(source/'hyjs-server',out/'native-source/hyjs-server')
  p=out/'native-source/hyjs-server/main.go';body=p.read_text();assert 'InitializeP14Qualification(ctx)' in body and 'ServeP14Qualification(ctx)' in body and 'serviceWorkerCount = 9' in body
  if '"strings"' not in body:body=body.replace('"hyjs-server/config"','"hyjs-server/config"\n"strings"',1)
  assert body.count('defer cancel()\n')==1
  body=body.replace('defer cancel()\n','defer cancel()\nif !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p13_qualified_native_") { return errors.New("P14 qualification native isolated schema required") }\n',1);p.write_text(body)
  for cmd,cwd in [(['gofmt','-w',str(p)],source),(['go','build','-o',str(out/'hyjs-native'),'.'],out/'native-source/hyjs-server'),(['go','test','-race','-c','-o',str(out/'service-race.test'),'./service'],source/'hyjs-server')]:
   r=subprocess.run(['rtk','proxy',*cmd],cwd=cwd,capture_output=True,text=True,timeout=300)
   if r.returncode:raise RuntimeError((r.stdout+r.stderr)[-7000:])
  meta={'files':{str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for folder in ['hyjs-server','analysis/tables'] for p in sorted((source/folder).rglob('*')) if p.is_file()},'nativeSHA256':hashlib.sha256((out/'hyjs-native').read_bytes()).hexdigest(),'raceSHA256':hashlib.sha256((out/'service-race.test').read_bytes()).hexdigest(),'scope':'root exactshared main/handler/domain source; disposable native main only adds strict ownschema guard'}
  (out/'manifest.json').write_text(json.dumps(meta,indent=2)+'\n');return out
 except BaseException:shutil.rmtree(out);raise
if __name__=='__main__':
 p=argparse.ArgumentParser();p.add_argument('--source',type=Path,required=True);a=p.parse_args();print(prepare(a.source))
