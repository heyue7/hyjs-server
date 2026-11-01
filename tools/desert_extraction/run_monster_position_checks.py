"""Exercise the collision position handshake against isolated Unity stubs."""
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
RUNTIME = Path('/Users/lirui/LR/hyjs/.tools/native-batch/dotnet')
APK = Path('/Users/lirui/LR/hyjs/deliveries/formal110/黑衣剑士灵魂之花v1.1.0.apk')

def run(args):
    subprocess.run(['rtk', 'proxy', *map(str, args)], check=True, timeout=90)

def main():
    dotnet = RUNTIME/'dotnet'
    compiler = next((RUNTIME/'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((RUNTIME/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    with tempfile.TemporaryDirectory(prefix='hyjs-monster-position-') as temporary:
        work = Path(temporary)
        stub = (HERE/'FieldBossPositionChecks.cs').read_text().split('public static class FieldBossPositionChecks')[0]
        (work/'Stubs.cs').write_text(stub)
        with zipfile.ZipFile(APK) as apk:
            (work/'LitJson.dll').write_bytes(apk.read('assets/bin/Data/Managed/LitJson.dll'))
        target = work/'MonsterChecks.dll'
        run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3', '/out:'+str(target),
             *['/reference:'+str(p) for p in sorted(refs.glob('*.dll'))], '/reference:'+str(work/'LitJson.dll'),
             work/'Stubs.cs', HERE/'FieldMonsterPosition.cs', HERE/'FieldMonsterPositionChecks.cs'])
        (work/'MonsterChecks.runtimeconfig.json').write_text(json.dumps({'runtimeOptions': {'tfm':'net8.0',
            'framework':{'name':'Microsoft.NETCore.App','version':refs.parents[1].name}}}))
        run([dotnet,target])

if __name__=='__main__':
    main()
