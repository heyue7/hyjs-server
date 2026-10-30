"""Check the source model, Unity 5.3 compilation and its existing public ABI."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import zipfile

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
EXPECTED = {
    'Assembly-CSharp.dll': '16432cee8041735e53af8254d226949cab594bd4be8bde1575d06522159233c8',
    'SoulFlowerDesertV4.dll': '432b79d0a4ef0997d525ca23cfb9f3ad0bde6c88f98a087711abea6fd12c6f62',
    'UnityEngine.dll': '8d4bd6321a453263f8f97047be50dd04a6f2b8f247456ad23fddfbc75f2c2fa4',
    'LitJson.dll': '151e0a2a6d174558ffa9cb681746d092fde6ba36e937b5825c2f06bb6a8d3230',
    'mscorlib.dll': '7f16171cf5f87ca1e4c229727526d2fbc786079fc0c960f36197013ea0d986e1',
    'System.dll': '7e72b92ddcd6b7b791ab24dd5756544f2da81f65e7ba7d1ebe8223123720d0ac',
    'System.Core.dll': '104278a0bf67b4de8fd1e910e3292a3ef51808ebee7a5f2a229125f18463c98c',
    'Assembly-CSharp-firstpass.dll': 'e732d075c1f52133f665e835ac623e32be89ec9aa2e22ca11925014dfdff95b8',
}
DEPENDENCY_STUBS = '''
namespace HYJS.DesertV4 {
 public sealed class FieldWire {
  public sealed class Session { public long playerId; public string sessionHex,origin; }
  public static bool Current(out Session value) { value=null; return false; }
 }
 public static class FieldRuntime { public static int SceneVersion; }
 public static class PositionSync { public static int ConnectionEpoch; }
}
'''
ABI_CHECK = '''
using System;
using System.Collections.Generic;
using dnlib.DotNet;
public static class ChipNamesAbi {
 static SortedSet<string> Read(string path) {
  ModuleDefMD module=ModuleDefMD.Load(path);
  TypeDef type=module.Find("HYJS.DesertV4.ChipNames",false);
  if (type==null) throw new Exception("ChipNames type missing: "+path);
  SortedSet<string> result=new SortedSet<string>();
  result.Add("type "+type.FullName+" "+(int)type.Attributes+" "+type.BaseType.FullName);
  foreach (MethodDef m in type.Methods) if (m.IsPublic)
   result.Add("method "+m.FullName+" "+(int)m.Attributes);
  foreach (FieldDef f in type.Fields) if (f.IsPublic)
   result.Add("field "+f.FullName+" "+(int)f.Attributes);
  return result;
 }
 public static void Main(string[] args) {
  SortedSet<string> before=Read(args[0]),after=Read(args[1]);
  if (!before.SetEquals(after)) {
   foreach (string item in before) if (!after.Contains(item)) Console.Error.WriteLine("removed "+item);
   foreach (string item in after) if (!before.Contains(item)) Console.Error.WriteLine("added "+item);
   throw new Exception("ChipNames public ABI differs");
  }
  Console.WriteLine("PASS "+before.Count+" ChipNames public type and method ABI entries match 1.0.88");
 }
}
'''


def run(command):
    subprocess.run(['rtk', 'proxy', *map(str, command)], check=True, timeout=90)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--game-apk', type=Path, default=ROOT/'build/hyjs-1.0.88-release.apk')
    parser.add_argument('--runtime', type=Path, default=Path('/Users/lirui/LR/hyjs/.tools/native-batch'))
    args = parser.parse_args()
    runtime = args.runtime/'dotnet'
    dotnet = runtime/'dotnet'
    csc = next((runtime/'sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = next((runtime/'packs/Microsoft.NETCore.App.Ref').glob('*/ref/net8.0'))
    dnlib = args.runtime/'injector/bin/Debug/net8.0/dnlib.dll'
    framework = refs.parents[1].name
    modern = ['/reference:' + str(path) for path in sorted(refs.glob('*.dll'))]
    with tempfile.TemporaryDirectory(prefix='hyjs-chip-names-check-') as tmp:
        work = Path(tmp)
        target = work/'ChipNamesChecks.dll'
        run([dotnet, csc, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(target), *modern, HERE/'ChipNames.cs', HERE/'ChipNamesChecks.cs'])
        runtime_config = json.dumps({'runtimeOptions': {
            'tfm': 'net8.0', 'framework': {'name': 'Microsoft.NETCore.App', 'version': framework}}})
        (work/'ChipNamesChecks.runtimeconfig.json').write_text(runtime_config)
        run([dotnet, target])
        with zipfile.ZipFile(args.game_apk) as apk:
            for name, expected in EXPECTED.items():
                raw = apk.read('assets/bin/Data/Managed/' + name)
                if hashlib.sha256(raw).hexdigest() != expected:
                    raise ValueError('Game ABI input differs: ' + name)
                (work/name).write_bytes(raw)
        (work/'Dependencies.cs').write_text(DEPENDENCY_STUBS)
        candidate = work/'ChipNamesUnity.dll'
        old = ['/reference:' + str(work/name) for name in EXPECTED if name != 'SoulFlowerDesertV4.dll']
        run([dotnet, csc, '/nologo', '/target:library', '/nostdlib+', '/langversion:3',
             '/out:' + str(candidate), *old, HERE/'ChipNames.cs', work/'Dependencies.cs'])
        print('PASS C# 3 source compiled against pinned 1.0.88 Unity and game assemblies', flush=True)
        shutil.copyfile(dnlib, work/'dnlib.dll')
        (work/'Abi.cs').write_text(ABI_CHECK)
        abi = work/'ChipNamesAbi.dll'
        run([dotnet, csc, '/nologo', '/target:exe', '/nostdlib+', '/langversion:3',
             '/out:' + str(abi), *modern, '/reference:' + str(dnlib), work/'Abi.cs'])
        (work/'ChipNamesAbi.runtimeconfig.json').write_text(runtime_config)
        run([dotnet, abi, work/'SoulFlowerDesertV4.dll', candidate])
    print('Temporary check files removed; no Unity scene or device executed')


if __name__ == '__main__':
    main()
