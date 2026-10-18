"""Build an in-memory native wallet session bridge without changing game logic."""
from pathlib import Path
import argparse
import hashlib
import json
import shutil
import subprocess
import sys
import tempfile
import dnfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT))
from tools.p10_voice.build import verify


def run(arguments):
    subprocess.run(['rtk', 'proxy', *map(str, arguments)], cwd=ROOT, check=True)


def verify_bridge_abi(path):
    """Keep the existing AssemblyRef and both injected/static call signatures."""
    assembly = dnfile.dnPE(str(path))
    if str(assembly.net.mdtables.Assembly.rows[0].Name) != 'LocalWalletSession':
        raise ValueError('Wallet bridge assembly identity changed')
    owner = next(t for t in assembly.net.mdtables.TypeDef if str(t.TypeName) == 'LocalWalletSessionBridge')
    signatures = {str(m.row.Name): m.row.Signature.value.hex() for m in owner.MethodList}
    required = {'Install': '000001', 'Pause': '000001',
                'RefreshAfterPayment': '2001010e', 'RefreshGameTime': '2001010e',
                'RefreshActivities': '2001010e'}
    for name, signature in required.items():
        if signatures.get(name) != signature:
            raise ValueError('Wallet bridge ABI changed: ' + name)
    return {'assemblyName': 'LocalWalletSession', 'requiredMethodSignatures': required}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--output-directory', type=Path, required=True)
    parser.add_argument('--bridge-only', action='store_true',
                        help='Recompile the same bridge ABI without touching the qualified game DLL')
    args = parser.parse_args()
    output = args.output_directory
    if output.exists() and any(output.iterdir()):
        raise ValueError('Output must be empty')
    output.mkdir(parents=True, exist_ok=True)
    work = ROOT / '.tools/native-batch'
    dotnet = work / 'dotnet/dotnet'
    compiler = next((work / 'dotnet/sdk').glob('*/Roslyn/bincore/csc.dll'))
    refs = sorted((ROOT / 'analysis/extracted/assets/bin/Data/Managed').glob('*.dll'))
    bridge = output / 'LocalWalletSession.dll'
    run([dotnet, compiler, '/nologo', '/target:library', '/nostdlib+', '/langversion:3', '/deterministic+',
         '/out:' + str(bridge), *['/reference:' + str(p) for p in refs],
         ROOT / 'tools/local_wallet_bridge/LocalWalletSessionBridge.cs'])
    abi = verify_bridge_abi(bridge)
    if args.bridge_only:
        proof = {'adapter': 'local-wallet-session-v1', 'mode': 'bridge-only', **abi,
                 'credentialsOnDisk': False, 'managedAssemblyUntouched': True,
                 'managedAssemblyOutputWritten': False, 'freshEngineComparison': False,
                 'engineProtectionBoundary': 'Caller must bind exact input APK to its prior qualified 547-method proof; no reinjection/recomparison claimed',
                 'inputSHA256': hashlib.sha256(args.input.read_bytes()).hexdigest(),
                 'outputSHA256': hashlib.sha256(args.input.read_bytes()).hexdigest(),
                 'bridgeSHA256': hashlib.sha256(bridge.read_bytes()).hexdigest()}
        (output / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
        print(json.dumps(proof))
        return
    with tempfile.TemporaryDirectory(prefix='hyjs-wallet-session-build-') as temporary:
        temporary = Path(temporary)
        shutil.copy2(ROOT / 'tools/local_wallet_bridge/Injector.cs', temporary / 'Program.cs')
        dnlib = work / 'injector/bin/Debug/net8.0/dnlib.dll'
        project = temporary / 'WalletSessionInjector.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType>'
                           '<TargetFramework>net8.0</TargetFramework><ImplicitUsings>enable</ImplicitUsings>'
                           '<Nullable>enable</Nullable></PropertyGroup><ItemGroup><Reference Include="dnlib">'
                           '<HintPath>' + str(dnlib) + '</HintPath></Reference></ItemGroup></Project>')
        run([dotnet, 'build', project, '--nologo', '--verbosity', 'quiet'])
        result = output / 'Assembly-CSharp.dll'
        run([dotnet, temporary / 'bin/Debug/net8.0/WalletSessionInjector.dll', args.input, bridge, result])
    proof = verify(args.input.read_bytes(), result.read_bytes(), {
        ('GameConfig', 'Awake'), ('LoginUserLogin', 'UserLogin'), ('LoginUI', 'OnSwitchAccountButtonClick')})
    proof.update(adapter='local-wallet-session-v1', mode='first-install', **abi, credentialsOnDisk=False,
                 inputSHA256=hashlib.sha256(args.input.read_bytes()).hexdigest(),
                 outputSHA256=hashlib.sha256(result.read_bytes()).hexdigest(),
                 bridgeSHA256=hashlib.sha256(bridge.read_bytes()).hexdigest())
    (output / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps(proof))


if __name__ == '__main__':
    main()
