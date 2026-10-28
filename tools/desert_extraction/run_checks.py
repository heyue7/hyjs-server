"""C#3 compatibility and gather race checks; performs no DB or network tests."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
APK_SHA = 'fe67aea7d584199119a04f10edcd8baa54207a70717e295d377ce3dd67b8affc'

def digest(path):
    h = hashlib.sha256()
    with path.open('rb') as f:
        for block in iter(lambda: f.read(1024*1024), b''): h.update(block)
    return h.hexdigest()

def run(args):
    result = subprocess.run(['rtk', 'proxy', *map(str,args)], text=True, capture_output=True, timeout=90)
    if result.returncode:
        raise RuntimeError(result.stdout + result.stderr)
    return result.stdout.strip()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base-apk', type=Path, required=True)
    parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    args=parser.parse_args()
    if digest(args.base_apk) != APK_SHA: raise ValueError('Expected pinned formal 1.0.78 APK')
    rt=args.runtime/'dotnet'; dotnet=rt/'dotnet'
    csc=next((rt/'sdk').glob('*/Roslyn/bincore/csc.dll'))
    ref=next((rt/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    framework=ref.parents[1].name
    with tempfile.TemporaryDirectory(prefix='hyjs-v4-client-check-') as tmp:
        tmp=Path(tmp)
        names=['Assembly-CSharp.dll','Assembly-CSharp-firstpass.dll','UnityEngine.dll','mscorlib.dll','System.dll','System.Core.dll','LitJson.dll']
        with zipfile.ZipFile(args.base_apk) as archive:
            for name in names: (tmp/name).write_bytes(archive.read('assets/bin/Data/Managed/'+name))
        oldrefs=['/reference:'+str(tmp/name) for name in names]
        sources=[HERE/name for name in ('GatherAttempt.cs','FieldWire.cs','GatherJournalStore.cs','FieldResources.cs','GatherPreload.cs','NativeGatherUI.cs','GatherCoordinator.cs','ProtectionRelease.cs','FieldRuntime.cs')]
        run([dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/out:'+str(tmp/'BridgeCheck.dll'),*oldrefs,*sources])
        (tmp/'EditorStub.cs').write_text('namespace UnityEditor {} namespace UnityEditor.SceneManagement { public static class EditorSceneManager { public static void OpenScene(string path) {} } }')
        run([dotnet,csc,'/nologo','/target:library','/nostdlib+','/langversion:3','/out:'+str(tmp/'AuditCheck.dll'),*oldrefs,tmp/'EditorStub.cs',HERE/'HyjsDesertV4PointAudit.cs',HERE/'HyjsDesertV4Suggest.cs',HERE/'HyjsDesertV4F09.cs',HERE/'HyjsDesertV4FishOptions.cs'])
        modernrefs=['/reference:'+str(p) for p in sorted(ref.glob('*.dll'))]
        test=tmp/'GatherAttemptChecks.dll'
        run([dotnet,csc,'/nologo','/target:exe','/nostdlib+','/langversion:3','/out:'+str(test),*modernrefs,HERE/'GatherAttempt.cs',HERE/'GatherAttemptChecks.cs'])
        (tmp/'GatherAttemptChecks.runtimeconfig.json').write_text(json.dumps({'runtimeOptions':{'tfm':'net8.0','framework':{'name':'Microsoft.NETCore.App','version':framework}}}))
        print(run([dotnet,test]))
    data=json.loads((HERE/'map-candidates.json').read_text())
    counts={k:sum(x['kind']==k for x in data['points']) for k in ('safe','mob','mine','fish','boss','exit')}
    if counts != {'safe':1,'mob':24,'mine':18,'fish':12,'boss':1,'exit':3}: raise ValueError('Candidate inventory differs')
    for point in data['points']:
        x=(point['pixelX']-256)/4.096+30.4; z=(256-point['pixelY'])/4.096+35.5
        if abs(x-point['worldX'])>1e-9 or abs(z-point['worldZ'])>1e-9: raise ValueError('Candidate transform differs')
    print('Unity 5.3 API + editor stub compile passed; candidate transform and inventory passed')
    print('No Unity scene, Android device, server, transport or database was executed')

if __name__=='__main__': main()
