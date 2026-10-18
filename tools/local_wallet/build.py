"""Build only classes2.dex + asset/manifest proposals, never an APK or install.

UnityPlayerActivity is compiled into a separate stub directory and used only as
the compiler classpath. D8 receives wallet class files exclusively; verify.py
rejects any defined Unity, Android or other unrelated class.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
_spec = importlib.util.spec_from_file_location('hyjs_local_wallet_verify', HERE / 'verify.py')
_verifier = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_verifier)
verify = _verifier.verify


def run(args, env=None):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], env=env,
                            capture_output=True, text=True, timeout=120)
    if result.returncode:
        raise RuntimeError('Wallet Java/D8 build failed: ' + result.stderr[-4000:])


def build(output, icon=None):
    output = Path(output).resolve()
    if not output.is_relative_to(ROOT / 'build') or output.exists():
        raise ValueError('Use a new owned project build output directory')
    sdk = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home() / 'Library/Android/sdk'))
    jdk = Path(os.environ.get('HYJS_WALLET_JAVA_HOME', '/opt/homebrew/opt/openjdk@21'))
    android = sdk / 'platforms/android-33/android.jar'
    # The bundled build-tools33 D8 predates the current JDK and fails on its
    # classpath metadata. Reuse the already installed current D8; no SDK install.
    d8 = sdk / 'cmdline-tools/latest/bin/d8'
    if not android.is_file() or not d8.is_file() or not (jdk / 'bin/javac').is_file():
        raise ValueError('Existing Android33 SDK and JDK21 are required; no automatic downloads')
    env = dict(os.environ, JAVA_HOME=str(jdk), PATH=str(jdk / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    with tempfile.TemporaryDirectory(prefix='hyjs-local-wallet-java-') as temporary:
        temp = Path(temporary)
        stubs, classes, dex = temp / 'stubs', temp / 'classes', temp / 'dex'
        for directory in (stubs, classes, dex):
            directory.mkdir()
        common = [jdk / 'bin/javac', '-encoding', 'UTF-8', '-source', '8', '-target', '8', '-bootclasspath', android]
        run([*common, '-d', stubs, *sorted((HERE / 'stubs').rglob('*.java'))], env)
        run([*common, '-classpath', stubs, '-d', classes, *sorted((HERE / 'src').rglob('*.java')), ROOT / 'tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java'], env)
        wallet_classes = sorted(classes.rglob('*.class'))
        if not wallet_classes or any(not str(path.relative_to(classes)).startswith('com/hyjs/wallet/') for path in wallet_classes):
            raise ValueError('Only wallet classes may be input to D8')
        run([d8, '--min-api', '23', '--lib', android, '--classpath', stubs, '--output', dex, *wallet_classes], env)
        if sorted(p.name for p in dex.iterdir()) != ['classes.dex']:
            raise ValueError('Wallet must fit one isolated dex')
        original_manifest = (ROOT / 'tools/AndroidManifest.xml').read_text()
        old = 'android:name="com.unity3d.player.UnityPlayerActivity"'
        if original_manifest.count(old) != 1:
            raise ValueError('Original single Unity launcher changed; root must review integration')
        manifest = original_manifest.replace(old, 'android:name="com.hyjs.wallet.WalletActivity"')
        output.mkdir(parents=True)
        (output / 'classes2.dex').write_bytes((dex / 'classes.dex').read_bytes())
        (output / 'AndroidManifest.wallet.xml').write_text(manifest)
        assets = output / 'assets'
        assets.mkdir()
        shutil.copy2(HERE / 'assets/local-wallet.json', assets / 'local-wallet.json')
        shutil.copy2(ROOT / 'tools/green_hero_compat/local-resource-overrides.json', assets / 'local-resource-overrides.json')
        icon = Path(icon) if icon else HERE / 'assets/icon.png'
        has_icon = icon.is_file()
        if has_icon:
            (assets / 'local-wallet').mkdir()
            shutil.copy2(icon, assets / 'local-wallet/icon.png')
        proof = verify(output / 'classes2.dex', output / 'AndroidManifest.wallet.xml')
        sources = sorted((HERE / 'src').rglob('*.java')) + sorted((HERE / 'stubs').rglob('*.java')) + [HERE / 'build.py', HERE / 'verify.py', HERE / 'assets/local-wallet.json', ROOT / 'tools/green_hero_compat/java/com/hyjs/wallet/LocalAssetOverlay.java', ROOT / 'tools/green_hero_compat/local-resource-overrides.json']
        proof.update(iconIncluded=has_icon, iconRequiredForFinalAPK=True,
                     iconSHA256=hashlib.sha256(icon.read_bytes()).hexdigest() if has_icon else None,
                     originalManifestSHA256=hashlib.sha256(original_manifest.encode()).hexdigest(),
                     sources=[{'path': str(p.relative_to(ROOT)), 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()} for p in sources],
                     temporaryCompilerIntermediatesCleaned=True,
                     packaging={'dexEntry': 'classes2.dex', 'assetEntries': ['assets/local-wallet.json', 'assets/local-resource-overrides.json'] + (['assets/local-wallet/icon.png'] if has_icon else []),
                                'manifest': 'AndroidManifest.wallet.xml', 'originalClassesDexMustRemainIdentical': True,
                                'monoBridgeRequired': 'WalletActivity.bindSession(long,String sessionHex)'})
        (output / 'manifest.json').write_text(json.dumps(proof, ensure_ascii=False, indent=2) + '\n')
        return proof


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--output', required=True)
    parser.add_argument('--icon')
    args = parser.parse_args()
    proof = build(args.output, args.icon)
    print(json.dumps({key: proof[key] for key in ['definedClassCount', 'dexSHA256', 'unityStubAbsent', 'iconIncluded', 'phoneUI']}, indent=2))
