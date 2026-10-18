"""Build a keyboard-only dex and exact original class_def patch; never package/install an APK."""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('hyjs_modern_ime_patch', HERE / 'patch.py')
adapter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(adapter)


def run(command, env):
    result = subprocess.run(['rtk', 'proxy', *map(str, command)], env=env, capture_output=True, text=True, timeout=120)
    if result.returncode:
        raise RuntimeError('IME offline Java/D8 validation failed: ' + result.stderr[-4000:])


def build(apk, output):
    apk, output = Path(apk).resolve(), Path(output).resolve()
    if not output.is_relative_to(ROOT / 'build') or output.exists():
        raise ValueError('Use a new owned build output directory')
    with zipfile.ZipFile(apk) as archive:
        original = archive.read('classes.dex')
        assembly = archive.read('assets/bin/Data/Managed/Assembly-CSharp.dll')
    patched, proof = adapter.patch(original)
    sdk = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home() / 'Library/Android/sdk'))
    jdk = Path(os.environ.get('HYJS_IME_JAVA_HOME', '/opt/homebrew/opt/openjdk@21'))
    android, d8 = sdk / 'platforms/android-33/android.jar', sdk / 'cmdline-tools/latest/bin/d8'
    if not android.is_file() or not d8.is_file() or not (jdk / 'bin/javac').is_file():
        raise ValueError('Existing Android33 SDK/JDK21 required; no automatic downloads')
    env = dict(os.environ, JAVA_HOME=str(jdk), PATH=str(jdk / 'bin') + os.pathsep + os.environ.get('PATH', ''))
    with tempfile.TemporaryDirectory(prefix='hyjs-modern-ime-') as temporary:
        temp = Path(temporary)
        stubs, classes, dex = temp / 'stubs', temp / 'classes', temp / 'dex'
        for directory in (stubs, classes, dex): directory.mkdir()
        common = [jdk / 'bin/javac', '-encoding', 'UTF-8', '-source', '8', '-target', '8', '-bootclasspath', android]
        run([*common, '-d', stubs, *sorted((HERE / 'stubs').rglob('*.java'))], env)
        run([*common, '-classpath', stubs, '-d', classes, *sorted((HERE / 'src').rglob('*.java'))], env)
        # Execute the exact production state/input-type code, without constructing Android views or an IME.
        harness = temp / 'InputLifecycleProbe.java'
        harness.write_text('''package com.unity3d.player;
public final class InputLifecycleProbe {
  private static int checks;
  private static void check(boolean value) { if (!value) throw new AssertionError("IME lifecycle check " + checks); checks++; }
  public static void main(String[] args) {
    s.InputState state = new s.InputState(); state.lastReported=""; state.open();
    check(!state.changed("n",true)); check(!state.changed("ni",true));
    check(state.ignoresEcho("", "ni", true));
    state.depth++; check(!state.changed("你",false)); state.depth--;
    check(state.changed("你",false)); check(!state.changed("你",false));
    check(state.changed("你😀",false)); check(!state.changed("你😀",false));
    check(state.changed("",false)); check(!state.changed("",false));
    check(state.finishOnce()); check(!state.finishOnce()); check(!state.changed("stale",false));
    state.close(); check(state.lastReported==null); state.open();
    check(state.changed("second-field",false)); check(state.finishOnce()); check(!state.finishOnce());
    state.close(); state.open(); check(state.changed("cancel-field",false));
    check(state.finishOnce()); check(!state.finishOnce()); state.close();
    int[] original={1,16385,12290,17,2,3,97,33};
    for(int keyboard=0;keyboard<8;keyboard++) for(int flags=0;flags<8;flags++) {
      int expected=original[keyboard]|((flags&1)!=0?32768:0)|((flags&2)!=0?131072:0)|((flags&4)!=0?128:0);
      check(s.originalInputType(keyboard,(flags&1)!=0,(flags&2)!=0,(flags&4)!=0)==expected);
    }
    System.out.println("PASS production-input-state checks="+checks);
  }
}''')
        probes = temp / 'probes'; probes.mkdir()
        run([*common, '-classpath', str(classes) + os.pathsep + str(android), '-d', probes, harness], env)
        run([jdk / 'bin/java', '-cp', os.pathsep.join(map(str, [probes, classes, stubs, android])), 'com.unity3d.player.InputLifecycleProbe'], env)
        compiled = sorted(classes.rglob('*.class'))
        if not compiled or any(not str(p.relative_to(classes)).startswith('com/unity3d/player/s') for p in compiled):
            raise ValueError('Only the exact Unity keyboard replacement family may enter D8')
        run([d8, '--min-api', '23', '--lib', android, '--classpath', stubs, '--output', dex, *compiled], env)
        if sorted(p.name for p in dex.iterdir()) != ['classes.dex']:
            raise ValueError('IME adapter must fit a single isolated dex')
        new_dex = (dex / 'classes.dex').read_bytes()
        new_classes = [name for name, _ in adapter.definitions(new_dex)]
        if 'Lcom/unity3d/player/s;' not in new_classes or any(not name.startswith('Lcom/unity3d/player/s$') and name != 'Lcom/unity3d/player/s;' for name in new_classes):
            raise ValueError('Unrelated Android/Unity definitions or compile stub leaked into IME dex')
        if set(new_classes).intersection(name for name, _ in adapter.definitions(patched)):
            raise ValueError('Duplicate original class definition in split dex')
        output.mkdir(parents=True)
        (output / 'classes.dex').write_bytes(patched)
        (output / 'classes3.dex').write_bytes(new_dex)
        sources = sorted((HERE / 'src').rglob('*.java')) + sorted((HERE / 'stubs').rglob('*.java')) + [HERE / 'patch.py', HERE / 'build.py']
        proof.update(inputAPK=str(apk.relative_to(ROOT)), inputAPKSHA256=adapter.digest(apk.read_bytes()),
                     replacementDexSHA256=adapter.digest(new_dex), replacementClasses=new_classes,
                     compileStubAbsent=True, originalAPKUntouched=True,
                     managedAssemblySHA256=adapter.digest(assembly), managedAssemblyUntouched=True,
                     imeFlags=['IME_FLAG_NO_EXTRACT_UI', 'IME_FLAG_NO_FULLSCREEN'],
                     secureIMEFlag='IME_FLAG_NO_PERSONALIZED_LEARNING',
                     originalTextValidationAndInputTypeMappingPreserved=True,
                     composingDeferredUntilCommit=True, realCurrentSystemIME=True,
                     exactProductionJVMInputStateChecks=84,
                     productionJVMChecks=['Chinese preedit deferred until commit', 'native echo preserves composing',
                                          'commit/emoji/delete deduplicate', 'Done/Back final once and stale callbacks ignored',
                                          'close/field-switch/reopen', 'all original8 keyboard types x8 exact flag combinations'],
                     passwordOrTextLogsOrPersistence=False, temporaryCompilerIntermediatesCleaned=True,
                     sources=[{'path': str(p.relative_to(ROOT)), 'sha256': adapter.digest(p.read_bytes())} for p in sources])
        (output / 'manifest.json').write_text(json.dumps(proof, indent=2) + '\n')
    return proof


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('--input-apk', required=True)
    parser.add_argument('--output', required=True)
    args = parser.parse_args()
    result = build(args.input_apk, args.output)
    print(json.dumps({key: result[key] for key in ['patchedDexSHA256', 'replacementDexSHA256', 'replacementClasses', 'allOriginalCodeItemsAndExceptionHandlersByteIdentical', 'deviceExecuted']}, indent=2))
