"""Compile the actual gather state, coordinator and journal store with test stubs."""
import argparse
import hashlib
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
APK_SHA = '5b8d368fad415204a4b9f67d512c36fe6614e4fe1055d1d362a93ec9dc26b695'

# Narrow transport/UI stubs; game types, LitJson and Unity come from the APK.
ACTUAL_GAME_STUBS = r'''
using System;
using LitJson;
using UnityEngine;
namespace HYJS.DesertV4 {
 public sealed class FieldWireResult { public bool uncertain; public long code; public string message; public JsonData data; }
 public sealed class FieldWire {
  public sealed class Session { public long playerId; public string sessionHex,origin; }
  public static bool Current(out Session s) { s=null; return false; }
  public void StartGather(Session s,GatherJournal j,Action<FieldWireResult> c) { }
  public void CancelGather(Session s,GatherJournal j,Action<FieldWireResult> c) { }
  public void ClaimGather(Session s,GatherJournal j,Action<FieldWireResult> c) { }
  public void GatherStatus(Session s,string id,Action<FieldWireResult> c) { }
 }
 public sealed class FieldResources {
  public sealed class Point { public string id,groupId,kind; public int remaining; public long generation; public Vector3 standPosition; public float radius; }
  public Point FindByNativePosId(string id) { return null; }
 }
 public sealed class GatherPreload { public bool Failed; public bool Ready(string kind) { return false; } }
 public sealed class NativeGatherUI {
  public bool Begin(FieldResources.Point p,GatherJournal j,Action c) { return false; }
  public bool IsAnimating() { return false; }
  public void Stop() { }
  public void CompleteFromNative() { }
 }
}'''


def run(args, env):
    subprocess.run(['rtk', 'proxy', *map(str, args)], env=env, check=True, timeout=90)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--runtime', type=Path, required=True, help='native-batch root')
    parser.add_argument('--apk', type=Path, help='optional pinned 1.1.4 APK for actual Unity/game references')
    args = parser.parse_args()
    runtime = args.runtime / 'dotnet'
    dotnet = runtime / 'dotnet'
    compiler = next((runtime / 'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime / 'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    env = dict(os.environ, DOTNET_PROCESSOR_COUNT='2', DOTNET_CLI_TELEMETRY_OPTOUT='1')
    modern = ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
    with tempfile.TemporaryDirectory(prefix='hyjs-gather-status-check-') as directory:
        temp = Path(directory)
        config = '{"runtimeOptions":{"tfm":"net8.0","framework":{"name":"Microsoft.NETCore.App","version":"%s"}}}' % refs.parents[1].name
        for name, sources in (
            ('GatherAttemptChecks', ('GatherAttempt.cs', 'GatherAttemptChecks.cs')),
            ('GatherCoordinatorChecks', ('GatherAttempt.cs', 'GatherJournalStore.cs', 'GatherCoordinator.cs', 'GatherCoordinatorChecks.cs')),
        ):
            output = temp / (name + '.dll')
            run([dotnet, compiler, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
                 '/out:' + str(output), *modern, *(HERE / source for source in sources)], env)
            (temp / (name + '.runtimeconfig.json')).write_text(config)
            run([dotnet, output], env)
        if args.apk:
            digest = hashlib.sha256()
            with args.apk.open('rb') as stream:
                for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                    digest.update(chunk)
            if digest.hexdigest() != APK_SHA:
                raise ValueError('Expected fixed 1.1.4 APK')
            names = ('Assembly-CSharp.dll', 'Assembly-CSharp-firstpass.dll', 'UnityEngine.dll',
                     'mscorlib.dll', 'System.dll', 'System.Core.dll', 'LitJson.dll',
                     'HYJS.Field63.dll', 'LocalFieldSettlement.dll')
            with zipfile.ZipFile(args.apk) as apk:
                for name in names:
                    (temp / name).write_bytes(apk.read('assets/bin/Data/Managed/' + name))
            stubs = temp / 'GameCompileStubs.cs'
            stubs.write_text(ACTUAL_GAME_STUBS)
            run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
                 '/out:' + str(temp / 'GatherActualReferences.dll'),
                 *['/reference:' + str(temp / name) for name in names], stubs,
                 HERE / 'GatherAttempt.cs', HERE / 'GatherJournalStore.cs', HERE / 'GatherCoordinator.cs'], env)
            print('Gather source compiled against pinned 1.1.4 Unity/game references')
    print('Temporary gather status check outputs cleaned')


if __name__ == '__main__':
    main()
