"""Compile fixed Voice native/race outside MySQL lock; native main refuses formal schemas."""
import argparse,hashlib,json,shutil,subprocess,tempfile
from pathlib import Path
def prepare(source):
    if not source.name.startswith('hyjs-p13-field-voice-'):raise AssertionError('fixed voice source guard')
    directory=Path(tempfile.mkdtemp(prefix='hyjs-p10-voice-prebuilt-'))
    try:
        native=directory/'native-source';shutil.copytree(source/'hyjs-server',native/'hyjs-server')
        path=native/'hyjs-server/main.go';main=path.read_text()
        if 'InitializeP10Voice(ctx)' not in main or 'ServeP10Voice(ctx)' not in main:raise AssertionError('actual root shared voice hooks required')
        main=main.replace('"hyjs-server/config"','"hyjs-server/config"\n"strings"',1)
        marker='defer cancel()\n';assert main.count(marker)==1
        main=main.replace(marker,marker+'if !strings.HasPrefix(os.Getenv("HYJS_MYSQL_DATABASE"), "hyjs_test_p10_") { return errors.New("voice native isolated schema required") }\n',1);path.write_text(main)
        subprocess.run(['rtk','proxy','gofmt','-w',str(path)],check=True,capture_output=True,text=True)
        subprocess.run(['rtk','proxy','go','build','-o',str(directory/'hyjs-native'),'.'],cwd=native/'hyjs-server',check=True,capture_output=True,text=True,timeout=180)
        subprocess.run(['rtk','proxy','go','test','-race','-c','-o',str(directory/'service-race.test'),'./service'],cwd=source/'hyjs-server',check=True,capture_output=True,text=True,timeout=240)
        files={str(p.relative_to(source)):hashlib.sha256(p.read_bytes()).hexdigest() for folder in ['hyjs-server','analysis/tables'] for p in sorted((source/folder).rglob('*')) if p.is_file()}
        (directory/'manifest.json').write_text(json.dumps({'files':files,'nativeSHA256':hashlib.sha256((directory/'hyjs-native').read_bytes()).hexdigest(),'raceSHA256':hashlib.sha256((directory/'service-race.test').read_bytes()).hexdigest(),'nativePatch':'only disposable main isolated-schema guard; all Voice endpoints and original dispatcher are root frozen shared hooks'},indent=2)+'\n');return directory
    except BaseException:shutil.rmtree(directory);raise
if __name__=='__main__':
    parser=argparse.ArgumentParser();parser.add_argument('--source-root',type=Path,required=True);args=parser.parse_args();print(prepare(args.source_root))
