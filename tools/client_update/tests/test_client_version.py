"""Compile and exercise the actual update bridge without an APK or network."""
import os
from pathlib import Path
import struct
import subprocess
import tempfile
import unittest
from unittest import mock
import zipfile

from tools.client_update import build


ROOT = Path(__file__).resolve().parents[3]
JAVA_SOURCE = ROOT / 'tools/client_update/src/com/hyjs/wallet'
TEST_SOURCE = Path(__file__).with_name('ClientVersionTest.java')
JDK = Path(os.environ.get('HYJS_WALLET_JAVA_HOME', '/opt/homebrew/opt/openjdk@21'))
SDK = Path(os.environ.get('ANDROID_SDK_ROOT', Path.home() / 'Library/Android/sdk'))
ANDROID = SDK / 'platforms/android-33/android.jar'


MOCKS = {
    'android/os/Looper.java': '''package android.os; public class Looper {
        public static Looper getMainLooper(){return new Looper();} }''',
    'android/os/Handler.java': '''package android.os; public class Handler {
        public Handler(Looper ignored){} public boolean post(Runnable r){r.run();return true;}
        public boolean postDelayed(Runnable r,long delay){return true;} public void removeCallbacks(Runnable r){} }''',
    'android/os/Build.java': '''package android.os; public class Build {
        public static class VERSION { public static int SDK_INT=33; } }''',
    'android/content/pm/PackageInfo.java': '''package android.content.pm; public class PackageInfo {
        public int versionCode; public long longVersionCode; public String versionName;
        public long getLongVersionCode(){return longVersionCode;} }''',
    'android/content/pm/PackageManager.java': '''package android.content.pm; public class PackageManager {
        private final PackageInfo info; public boolean fail;
        public PackageManager(PackageInfo value){info=value;}
        public PackageInfo getPackageInfo(String name,int flags){if(fail)throw new RuntimeException();return info;} }''',
    'android/content/Context.java': '''package android.content; public class Context {}''',
    'android/app/Activity.java': '''package android.app; import android.content.pm.PackageManager;
        public class Activity extends android.content.Context { private final PackageManager manager; public Activity(PackageManager value){manager=value;}
        public PackageManager getPackageManager(){return manager;} public String getPackageName(){return "test";} }''',
    'com/unity3d/player/UnityPlayer.java': '''package com.unity3d.player; import android.app.Activity;
        public class UnityPlayer { public static Activity currentActivity; }''',
    'com/hyjs/wallet/WalletActivity.java': '''package com.hyjs.wallet; public class WalletActivity {
        public static final Object credentials=new Credentials();
        private static final class Credentials { private long player=42; private String session="0123456789abcdef0123456789abcdef"; } }''',
    'org/json/JSONObject.java': '''package org.json; import java.util.*;
        public class JSONObject { private final Map<String,Object> data=new LinkedHashMap<String,Object>();
        public JSONObject(){} public JSONObject(String value){data.put("code",0);data.put("data",new JSONObject());}
        public JSONObject put(String k,Object v){data.put(k,v);return this;}
        public JSONObject put(String k,long v){data.put(k,v);return this;}
        public JSONObject put(String k,boolean v){data.put(k,v);return this;}
        public boolean has(String k){return data.containsKey(k);}
        public long getLong(String k){return ((Number)data.get(k)).longValue();}
        public String getString(String k){return String.valueOf(data.get(k));}
        public JSONObject getJSONObject(String k){return (JSONObject)data.get(k);}
        public int optInt(String k,int fallback){Object v=data.get(k);return v instanceof Number?((Number)v).intValue():fallback;}
        public boolean optBoolean(String k,boolean fallback){Object v=data.get(k);return v instanceof Boolean?(Boolean)v:fallback;}
        public String toString(){StringBuilder b=new StringBuilder("{");boolean first=true;
            for(Map.Entry<String,Object> e:data.entrySet()){if(!first)b.append(',');first=false;
                b.append('"').append(e.getKey()).append("\\\":");Object v=e.getValue();
                if(v instanceof String)b.append('"').append(v).append('"');else b.append(v);}
            return b.append('}').toString();} }''',
}


def run(*args):
    result = subprocess.run(['rtk', 'proxy', *map(str, args)], cwd=ROOT, capture_output=True, text=True, timeout=120)
    if result.returncode:
        raise AssertionError('command failed: ' + str(args[0]) + '\n' + result.stdout + result.stderr)


def dex_classes(raw):
    _, strings_offset, _, types_offset = struct.unpack_from('<4I', raw, 56)
    count, offset = struct.unpack_from('<2I', raw, 96)
    names = set()
    for index in range(count):
        type_index = struct.unpack_from('<I', raw, offset + 32 * index)[0]
        string_index = struct.unpack_from('<I', raw, types_offset + 4 * type_index)[0]
        cursor = struct.unpack_from('<I', raw, strings_offset + 4 * string_index)[0]
        while raw[cursor] & 128: cursor += 1
        cursor += 1
        names.add(raw[cursor:raw.index(0, cursor)].decode('ascii'))
    return names


class ClientVersionTests(unittest.TestCase):
    def test_build_input_validation(self):
        for value in ('0.0.0', '1.1.0', '1.10.0', '999.999.999'):
            self.assertTrue(build.valid_version_name(value), value)
        for value in ('', '1', '1.2', '1.2.3.4', '1..0', '.1.0', '1.0.', '01.0.0', '0.00.0', '0.0.000',
                      '1000.0.0', '-1.0.0', '+1.0.0', ' 1.0.0', '1.0.0 ', '1.١.0', '1.0.0-beta'):
            self.assertFalse(build.valid_version_name(value), value)
        with tempfile.TemporaryDirectory(prefix='hyjs-client-update-cli-') as temp:
            result = subprocess.run(['rtk', 'proxy', os.sys.executable, '-B', str(ROOT / 'tools/client_update/build.py'),
                                     '--base-apk', str(Path(temp) / 'missing.apk'), '--base-sha256', 'unused',
                                     '--build-code', '1', '--version-name', '01.0.0', '--output-directory', str(Path(temp) / 'out')],
                                    cwd=ROOT, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('three canonical ASCII integer segments', result.stderr)
            self.assertFalse((Path(temp) / 'out').exists())

    def test_new_version_order(self):
        for value, base in (('1.10.0', '1.9.999'), ('2.0.0', '1.999.999'),
                            ('0.0.1', '0.0.0'), ('0.0.0', 'legacy'), ('1.0.0', '01.0.0')):
            self.assertTrue(build.version_name_advances(value, base), (value, base))
        for value, base in (('1.9.999', '1.9.999'), ('1.9.998', '1.9.999'),
                            ('1.9.999', '1.10.0'), ('1.999.999', '2.0.0'),
                            ('999.999.999', '999.999.999'), ('01.0.0', 'legacy')):
            self.assertFalse(build.version_name_advances(value, base), (value, base))

    def test_java_dex_and_runtime(self):
        self.assertTrue(ANDROID.is_file(), 'Android SDK 33 is required for the production Java build')
        with tempfile.TemporaryDirectory(prefix='hyjs-client-update-test-') as name:
            temp = Path(name)
            out, classes, stubs, mocks, mock_source = (temp / p for p in ('dex-out', 'classes', 'stubs', 'mocks', 'mock-source'))
            for directory in (out, classes, stubs, mocks): directory.mkdir()
            # Invoke the actual packaging compiler; every class from ClientUpdate.java must reach the dex.
            build.build_java(out, 2026110107, '1.2.6')
            defined = dex_classes((out / 'classes.dex').read_bytes())
            for descriptor in ('Lcom/hyjs/wallet/ClientUpdate;', 'Lcom/hyjs/wallet/ClientVersion;',
                               'Lcom/hyjs/wallet/ClientUpdateActivity;'):
                self.assertIn(descriptor, defined)
            self.assertNotIn('Lcom/hyjs/wallet/WalletActivity;', defined)

            stub = temp / 'WalletActivity.java'
            stub.write_text('package com.hyjs.wallet; public class WalletActivity extends android.app.Activity {}')
            javac = JDK / 'bin/javac'
            run(javac, '--release', '8', '-classpath', ANDROID, '-d', stubs, stub)
            run(javac, '-encoding', 'UTF-8', '--release', '8', '-classpath', str(ANDROID) + os.pathsep + str(stubs),
                '-d', classes, JAVA_SOURCE / 'ClientUpdate.java', JAVA_SOURCE / 'ClientUpdateActivity.java')
            mock_files = []
            for relative, source in MOCKS.items():
                file = mock_source / relative
                file.parent.mkdir(parents=True, exist_ok=True)
                file.write_text(source)
                mock_files.append(file)
            run(javac, '--release', '8', '-d', mocks, *mock_files)
            classpath = os.pathsep.join(map(str, (mocks, classes, ANDROID)))
            run(javac, '-encoding', 'UTF-8', '--release', '8', '-classpath', classpath, '-d', mocks, TEST_SOURCE)
            result = subprocess.run(['rtk', 'proxy', str(JDK / 'bin/java'), '-classpath', classpath,
                                     'com.hyjs.wallet.ClientVersionTest'], cwd=ROOT, capture_output=True, text=True, timeout=30)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertIn('ClientVersionTest passed', result.stdout)

    @unittest.skipUnless(os.environ.get('HYJS_CLIENT_UPDATE_TEST_APK'), 'set HYJS_CLIENT_UPDATE_TEST_APK to verify an actual base manifest')
    def test_actual_manifest_version_injection(self):
        from tools.client_update.package import patch_manifest
        with zipfile.ZipFile(os.environ['HYJS_CLIENT_UPDATE_TEST_APK']) as apk:
            original = apk.read('AndroidManifest.xml')
        base_code, _ = build.manifest_versions(original)
        revised = patch_manifest(original, base_code + 1, '1.10.0')
        self.assertEqual(build.manifest_versions(revised), (base_code + 1, '1.10.0'))
        with self.assertRaises(Exception):
            patch_manifest(original, base_code, '1.10.0')

        # Use an actual binary manifest in a tiny fixture; stop before any output is built.
        with tempfile.TemporaryDirectory(prefix='hyjs-client-update-order-') as name:
            temp = Path(name)
            fixture = temp / 'base.apk'
            def set_base(version):
                with zipfile.ZipFile(fixture, 'w') as apk:
                    apk.writestr('AndroidManifest.xml', patch_manifest(original, base_code + 1, version))
            def arguments(version, build_code):
                return [str(ROOT / 'tools/client_update/build.py'), '--base-apk', str(fixture),
                        '--base-sha256', build.sha(fixture.read_bytes()), '--build-code', str(build_code),
                        '--version-name', version, '--output-directory', str(temp / 'out')]
            set_base('1.9.999')
            for version in ('1.9.999', '1.9.998'):
                result = subprocess.run(['rtk', 'proxy', os.sys.executable, '-B', *arguments(version, base_code + 2)],
                                        cwd=ROOT, capture_output=True, text=True, timeout=30)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('version name must exceed the base manifest version', result.stderr)
                self.assertFalse((temp / 'out').exists())
            # The actual entry point accepts a numerical upgrade, then reaches the build step.
            with mock.patch.object(os.sys, 'argv', arguments('1.10.0', base_code + 2)), \
                    mock.patch.object(Path, 'mkdir', side_effect=RuntimeError('past-version-check')):
                with self.assertRaisesRegex(RuntimeError, 'past-version-check'):
                    build.main()
            # An unknown legacy baseline still accepts a valid target name.
            set_base('legacy-version')
            with mock.patch.object(os.sys, 'argv', arguments('0.0.0', base_code + 2)), \
                    mock.patch.object(Path, 'mkdir', side_effect=RuntimeError('past-version-check')):
                with self.assertRaisesRegex(RuntimeError, 'past-version-check'):
                    build.main()
            # Even for an unknown baseline name, Android versionCode must increase.
            result = subprocess.run(['rtk', 'proxy', os.sys.executable, '-B', *arguments('0.0.0', base_code + 1)],
                                    cwd=ROOT, capture_output=True, text=True, timeout=30)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('build code must exceed the base manifest', result.stderr)
            self.assertFalse((temp / 'out').exists())


if __name__ == '__main__':
    unittest.main()
