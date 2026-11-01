"""Exercise the original request's bounded retry with a simulated HTTP channel."""
import json
from pathlib import Path
import subprocess
import tempfile

HERE=Path(__file__).resolve().parent
RUNTIME=Path('/Users/lirui/LR/hyjs/.tools/native-batch/dotnet')

def run(args):
    subprocess.run(['rtk','proxy',*map(str,args)],check=True,timeout=90)

def main():
    compiler=next((RUNTIME/'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs=next((RUNTIME/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    with tempfile.TemporaryDirectory(prefix='hyjs-expedition-retry-') as temporary:
        work=Path(temporary); target=work/'Checks.dll'
        run([RUNTIME/'dotnet',compiler,'/nologo','/target:exe','/nostdlib+','/langversion:3','/out:'+str(target),
             *['/reference:'+str(p) for p in sorted(refs.glob('*.dll'))],HERE/'ExpeditionHeroList.cs',HERE/'ExpeditionHeroListChecks.cs'])
        (work/'Checks.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0',
            'framework':{'name':'Microsoft.NETCore.App','version':refs.parents[1].name}}}))
        run([RUNTIME/'dotnet',target])

if __name__=='__main__':
    main()
