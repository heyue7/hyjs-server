"""Install and inspect the local Android demo without retaining image payloads in chat."""
import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import re
import socket
import tempfile
import time
import xml.etree.ElementTree as ET

from PIL import Image, ImageStat

ROOT = Path(__file__).resolve().parents[1]
ADB = Path.home() / 'Library/Android/sdk/platform-tools/adb'
PACKAGE = 'com.yunchang.djsy.ewan.local'
MULTI_PACKAGE = 'com.yunchang.djsy.ewan.multi'
DISPLAY_STATE = ROOT / '.local-env/device-display.json'


def select_device(serial):
    if serial:
        return serial
    result = subprocess.run([str(ADB), 'devices'], check=True, capture_output=True,
                            text=True, timeout=15)
    devices = [line.split()[0] for line in result.stdout.splitlines()[1:]
               if len(line.split()) >= 2 and line.split()[1] == 'device']
    if not devices:
        raise RuntimeError('No connected authorized Android device')
    # Prefer the existing USB transport when both transports identify the same phone.
    wired = [value for value in devices if ':' not in value]
    if len(wired) == 1:
        return wired[0]
    if len(devices) == 1:
        return devices[0]
    raise RuntimeError('Multiple phones connected; supply --serial')


class Device:
    def __init__(self, serial, package=PACKAGE):
        if package not in (PACKAGE, MULTI_PACKAGE):
            raise ValueError('Use an explicitly owned local acceptance package')
        self.serial = select_device(serial)
        self.package = package

    def display_state_file(self):
        # Keep the physical phone's saved preferences when a disposable emulator
        # is used in parallel for client-flow checks.
        if self.serial.startswith('emulator-'):
            return ROOT / '.local-env' / ('device-display-' + self.serial + '.json')
        return DISPLAY_STATE

    def run(self, *args, timeout=20):
        try:
            result = subprocess.run([str(ADB), '-s', self.serial, *map(str, args)],
                                    capture_output=True, timeout=timeout)
        except subprocess.TimeoutExpired:
            # A login text command may contain a saved credential. Report the
            # transport timeout without exposing argv in Python's traceback.
            raise RuntimeError('Android command timed out; inspect the existing '
                               'device state before retrying') from None
        if result.returncode:
            raise RuntimeError(result.stderr.decode(errors='replace')[:400])
        return result.stdout

    def text(self, *args, timeout=20):
        return self.run(*args, timeout=timeout).decode(errors='replace').strip()

    def status(self):
        power = self.text('shell', 'dumpsys', 'power')
        activity = self.text('shell', 'dumpsys', 'activity', 'activities')
        selected = [line.strip() for line in power.splitlines()
                    if line.strip().startswith(('mWakefulness=', 'mIsPowered=',
                                                'mStayOn=', 'mHalInteractiveModeEnabled='))]
        resumed = [line.strip() for line in activity.splitlines()
                   if 'topResumedActivity=' in line or 'mResumedActivity:' in line]
        return {'serial': self.serial, 'power': selected, 'foreground': resumed,
                'keyguard': self.keyguard(),
                'reverse': self.text('reverse', '--list').splitlines()}

    def connect_local(self, http_port, tcp_port):
        # USB重连会丢失reverse；两个原通道都可达后才返回，避免HTTP登录成功但场景TCP断开。
        ports = [http_port, tcp_port]
        if any(type(port) is not int or not 1 <= port <= 65535 for port in ports) or http_port == tcp_port:
            raise ValueError('Use two distinct valid local game ports')
        self.text('shell', 'pm', 'path', self.package)
        bindings = {}
        for line in self.text('reverse', '--list').splitlines():
            fields = line.split()
            if len(fields) == 3:
                bindings[fields[1]] = fields[2]
        # 先检查全部现有归属和本机监听，再修改明确的两个转发；不覆盖其他服务映射。
        for port in ports:
            endpoint = f'tcp:{port}'
            if endpoint in bindings and bindings[endpoint] != endpoint:
                raise RuntimeError('A requested phone port already forwards to another service')
            try:
                with socket.create_connection(('127.0.0.1', port), timeout=2):
                    pass
            except OSError:
                raise RuntimeError(f'Local game port {port} is unavailable') from None
        for port in ports:
            endpoint = f'tcp:{port}'
            if endpoint not in bindings:
                self.run('reverse', '--no-rebind', endpoint, endpoint)
        actual = self.text('reverse', '--list').splitlines()
        if any(not any(line.split()[-2:] == [f'tcp:{port}', f'tcp:{port}'] for line in actual)
               for port in ports):
            raise RuntimeError('Phone game port forwarding could not be verified')
        return {'http_port': http_port, 'tcp_port': tcp_port, 'transport': 'verified'}

    def keyguard(self):
        policy = self.text('shell', 'dumpsys', 'window', 'policy')
        section = policy.partition('KeyguardServiceDelegate')[2].partition('KeyguardStateMonitor')[0]
        state = {}
        for key in ('showing', 'secure', 'occluded'):
            match = re.search(r'^\s*' + key + r'=(true|false)\s*$', section, re.M)
            state[key] = match.group(1) == 'true' if match else None
        return state

    def game_is_foreground(self, activity=None):
        if activity is None:
            activity = self.text('shell', 'dumpsys', 'activity', 'activities')
        target = re.compile(r'(?<![A-Za-z0-9_.])' + re.escape(self.package) + r'(?=$|[/\s}])')
        return any(target.search(line) for line in activity.splitlines()
                   if any(marker in line for marker in
                          ('topResumedActivity=', 'mResumedActivity:', 'mResumedActivity=')))

    def require_game_surface_accessible(self):
        state = self.keyguard()
        if state['showing'] and state['secure']:
            # Android permits an explicitly showWhenLocked activity above a
            # secure keyguard. Accept only this foreground demo, never the lock UI.
            if state['occluded'] and self.game_is_foreground():
                return
            raise RuntimeError('Phone system lock requires the owner to unlock once; '
                               'keep stayon enabled afterwards. No PIN is collected.')

    def prepare(self):
        # Save display preferences once, then keep the unattended phone awake while testing.
        display_state = self.display_state_file()
        current_timeout = self.text('shell', 'settings', 'get', 'system', 'screen_off_timeout')
        if not display_state.exists():
            state = {'serial': self.serial,
                     'stay_on': self.text('shell', 'settings', 'get', 'global',
                                          'stay_on_while_plugged_in'),
                     'screen_timeout': current_timeout}
            display_state.parent.mkdir(exist_ok=True)
            fd = os.open(display_state, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, 'w') as file:
                json.dump(state, file)
        self.run('shell', 'svc', 'power', 'stayon', 'true')
        # Keep the owner's longer timeout intact when restarting or installing.
        try:
            timeout = int(current_timeout)
        except ValueError:
            timeout = 0
        if 0 <= timeout < 1800000:
            self.run('shell', 'settings', 'put', 'system', 'screen_off_timeout', '1800000')
        self.run('shell', 'input', 'keyevent', 'KEYCODE_WAKEUP')
        # Wake only; never deliberately sleep during unattended work. A secure
        # keyguard needs the owner, while a nonsecure swipe lock can be dismissed.
        state = self.keyguard()
        if state['showing'] and state['secure'] is False:
            self.run('shell', 'wm', 'dismiss-keyguard')

    def frame(self):
        raw = self.run('exec-out', 'screencap', '-p')
        return Image.open(io.BytesIO(raw)).convert('RGB')

    def capture(self, output):
        frame = self.frame()
        output = Path(output).resolve()
        output.parent.mkdir(parents=True, exist_ok=True)
        frame.save(output)
        preview = output.with_name(output.stem + '-preview.jpg')
        frame.thumbnail((1280, 720))
        frame.save(preview, quality=70, optimize=True)
        return {'evidence': str(output), 'preview': str(preview),
                'preview_bytes': preview.stat().st_size}

    def ocr(self, contains=''):
        source = ROOT / 'tools/device_ocr.m'
        binary = ROOT / 'build/hyjs-device-ocr'
        if not binary.exists() or binary.stat().st_mtime < source.stat().st_mtime:
            compiled = subprocess.run(['clang', '-fobjc-arc', '-ferror-limit=3',
                                       '-framework', 'Foundation', '-framework', 'Vision',
                                       '-framework', 'CoreGraphics', str(source), '-o', str(binary)],
                                      capture_output=True, timeout=40)
            if compiled.returncode:
                raise RuntimeError('Local screenshot OCR compiler is unavailable')
        # Use a temporary local image; emit only selected text and coordinates to chat.
        frame = self.frame()
        frame.thumbnail((1920, 1080))
        with tempfile.NamedTemporaryFile(suffix='.jpg') as file:
            frame.save(file.name, quality=85)
            try:
                result = subprocess.run([str(binary), file.name, contains], capture_output=True,
                                        text=True, timeout=15)
            except subprocess.TimeoutExpired:
                raise RuntimeError('Local screenshot OCR timed out; inspect a compact screenshot') from None
            if result.returncode == 124:
                raise RuntimeError('Local screenshot OCR timed out; inspect a compact screenshot')
            if result.returncode:
                raise RuntimeError('Local screenshot OCR failed; inspect a compact screenshot')
            try:
                entries = json.loads(result.stdout)
                if not isinstance(entries, list):
                    raise ValueError('Expected text entries')
                for entry in entries:
                    if (not isinstance(entry, dict) or not isinstance(entry.get('text'), str)
                            or any(not isinstance(entry.get(key), (int, float))
                                   or not 0 <= entry[key] <= 1
                                   for key in ('confidence', 'x', 'y', 'width', 'height'))):
                        raise ValueError('Invalid text coordinates')
            except (ValueError, TypeError, KeyError):
                raise RuntimeError('Local screenshot OCR returned invalid coordinates; '
                                   'inspect a compact screenshot') from None
            return entries

    def tap_text(self, text, offset_x=0, offset_y=0):
        self.require_game_surface_accessible()
        activity = self.text('shell', 'dumpsys', 'activity', 'activities')
        if not self.game_is_foreground(activity):
            raise RuntimeError('Text actions are scoped to the foreground demo game')
        matches = [item for item in self.ocr(text)
                   if item['text'].strip() == text and item['confidence'] >= 0.4]
        if len(matches) != 1:
            raise RuntimeError('No unique verified text button; inspect the current UI')
        frame = self.frame()
        item = matches[0]
        x, y = item['x'] + offset_x, item['y'] + offset_y
        if not (0 <= x <= 1 and 0 <= y <= 1):
            raise RuntimeError('Text action offset is outside the observed viewport')
        self.run('shell', 'input', 'tap', round(x * frame.width), round(y * frame.height))
        return {'text_tapped': text}

    def wait_rendered(self, timeout):
        # A bounded render check distinguishes a sleeping/black surface from loaded UI.
        deadline = time.monotonic() + timeout
        self.require_game_surface_accessible()
        last = {}
        while True:
            frame = self.frame()
            frame.thumbnail((160, 100))
            grey = frame.convert('L')
            stats = ImageStat.Stat(grey)
            bright = sum(value > 20 for value in grey.tobytes()) / (grey.width * grey.height)
            activity = self.text('shell', 'dumpsys', 'activity', 'activities')
            foreground = self.game_is_foreground(activity)
            last = {'game_foreground': foreground, 'mean_luminance': round(stats.mean[0], 2),
                    'bright_fraction': round(bright, 3)}
            if foreground and bright > 0.08 and stats.stddev[0] > 3:
                return last
            if time.monotonic() >= deadline:
                raise RuntimeError('Game surface did not render within deadline: ' + json.dumps(last))
            time.sleep(1)

    def restart(self):
        self.prepare()
        # Resolve this installed package's launcher so the optional native
        # wallet activity and the original Unity activity use the same driver.
        resolved = self.text('shell', 'cmd', 'package', 'resolve-activity', '--brief',
                             '-a', 'android.intent.action.MAIN',
                             '-c', 'android.intent.category.LAUNCHER', self.package)
        components = [line.strip() for line in resolved.splitlines()
                      if line.strip().startswith(self.package + '/') and ' ' not in line.strip()]
        if len(components) != 1:
            raise RuntimeError('No unique installed launcher for the selected game package')
        self.run('shell', 'am', 'force-stop', self.package)
        self.run('shell', 'am', 'start', '-n', components[0])

    def approve_own_install(self):
        # Vivo may hold an authorized ADB install for its on-device confirmation UI.
        # Act only on the installer surface that names this local demo application.
        activity = self.text('shell', 'dumpsys', 'activity', 'activities')
        current = [line for line in activity.splitlines() if 'topResumedActivity=' in line]
        if not any('packageinstaller' in line.lower() for line in current):
            return False
        self.run('shell', 'uiautomator', 'dump', '/sdcard/hyjs-installer.xml')
        tree = ET.fromstring(self.run('shell', 'cat', '/sdcard/hyjs-installer.xml'))
        nodes = list(tree.iter('node'))
        if not any('黑衣剑士·自建服' in node.get('text', '') for node in nodes):
            return False
        for expected in ['已了解应用的风险检测结果', '继续安装', '重新安装', '安装', '允许']:
            for node in nodes:
                if node.get('text') != expected or node.get('enabled') != 'true':
                    continue
                if expected.startswith('已了解') and (node.get('checked') == 'true' or
                        any(item.get('checkable') == 'true' and item.get('checked') == 'true'
                            for item in nodes)):
                    continue
                coords = list(map(int, re.findall(r'\d+', node.get('bounds', ''))))
                if len(coords) == 4:
                    self.run('shell', 'input', 'tap', (coords[0] + coords[2]) // 2,
                             (coords[1] + coords[3]) // 2)
                    return True
        return False

    def install(self, apk):
        apk = Path(apk).resolve()
        with apk.open('rb') as file:
            expected = hashlib.file_digest(file, 'sha256').hexdigest()
        suffix = '-multi' if self.package == MULTI_PACKAGE else ''
        record = ROOT / '.local-env' / ('owned-install-' + self.serial.replace(':', '-') + suffix + '.json')
        if record.exists():
            raise RuntimeError('A previous owned install session is saved; inspect that same '
                               'session before starting another installation')
        if self.package == MULTI_PACKAGE:
            aapt = Path.home()/'Library/Android/sdk/build-tools/33.0.0/aapt2'
            result = subprocess.run(['rtk', 'proxy', str(aapt), 'dump', 'badging', str(apk)],
                                    capture_output=True, text=True, timeout=30, check=True)
            match = re.search(r"^package: name='([^']+)'", result.stdout, re.M)
            if match is None or match.group(1) != self.package:
                raise RuntimeError('Secondary install requires the exact owned multi-client package')
        self.prepare()
        # Create an explicit owned session. A stalled vendor callback may be
        # recommitted once using this same ID without starting another install.
        temporary = '/data/local/tmp/hyjs-own-' + expected[:16] + '.apk'
        self.run('push', apk, temporary, timeout=60)
        created = self.text('shell', 'cmd', 'package', 'install-create', '-r',
                            '-S', apk.stat().st_size)
        match = re.fullmatch(r'Success: created install session \[(\d+)\]', created)
        if not match:
            raise RuntimeError('Android did not create a verifiable owned install session')
        session = match.group(1)
        record.parent.mkdir(parents=True, exist_ok=True)
        record.write_text(json.dumps({'sessionId': session, 'apkSHA256': expected,
                                      'temporaryPath': temporary}) + '\n')
        self.text('shell', 'cmd', 'package', 'install-write', '-S', apk.stat().st_size,
                  session, 'base.apk', temporary, timeout=60)
        command = [str(ADB), '-s', self.serial, 'shell', 'cmd', 'package', 'install-commit', session]
        proc = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        started = time.monotonic()
        recommitted = False
        while proc.poll() is None:
            self.approve_own_install()
            elapsed = time.monotonic() - started
            if elapsed >= 35 and not recommitted:
                recommitted = True
                try:
                    self.text('shell', 'cmd', 'package', 'install-commit', session, timeout=45)
                except RuntimeError:
                    pass  # Resolve an ambiguous callback by the actual APK hash.
            if elapsed >= 40:
                try:
                    path = self.text('shell', 'pm', 'path', self.package).split(':', 1)[1]
                    applied = self.text('shell', 'sha256sum', path, timeout=40).split()[0]
                except (RuntimeError, IndexError):
                    applied = None
                if applied == expected:
                    proc.terminate()
                    break
            if elapsed >= 120:
                proc.terminate()
                proc.communicate(timeout=10)
                raise RuntimeError('Owned install session remains pending; inspect its saved '
                                   'session ID before retrying')
            time.sleep(2)
        stdout, stderr = proc.communicate(timeout=10)
        # vivo 可能在已提交更新后返回 INSTALL_FAILED_ABORTED；先核对实际包。
        # 仅完全相同的候选 SHA 允许继续，避免重复启动已成功的安装会话。
        try:
            package_path = self.text('shell', 'pm', 'path', self.package).split(':', 1)[1]
            actual = self.text('shell', 'sha256sum', package_path, timeout=40).split()[0]
        except (RuntimeError, IndexError):
            raise RuntimeError('Android candidate is not installed; inspect the completed installer') from None
        if actual != expected:
            if proc.returncode or b'Success' not in stdout:
                raise RuntimeError('Android installation failed; installed APK differs from candidate')
            raise RuntimeError('Installed APK does not match the candidate SHA-256')
        self.run('shell', 'rm', '-f', temporary)
        record.unlink(missing_ok=True)
        self.restart()
        return {'install': 'Success', 'apk_sha256': actual,
                'render': self.wait_rendered(40)}

    def restore_display(self):
        display_state = self.display_state_file()
        state = json.loads(display_state.read_text())
        # Restore only the preferences this tool changed; no account data is touched.
        for namespace, key, value in [('global', 'stay_on_while_plugged_in', state['stay_on']),
                                       ('system', 'screen_off_timeout', state['screen_timeout'])]:
            if value == 'null':
                self.run('shell', 'settings', 'delete', namespace, key)
            else:
                self.run('shell', 'settings', 'put', namespace, key, value)
        display_state.unlink()

    def login_saved(self, account):
        self.require_game_surface_accessible()
        activity = self.text('shell', 'dumpsys', 'activity', 'activities')
        if not self.game_is_foreground(activity):
            raise RuntimeError('Saved credentials can only be entered in the foreground demo')
        account = Path(account).resolve()
        if account.parent != ROOT / '.local-env':
            raise RuntimeError('Use a local credential file under .local-env')
        frame = self.frame()
        if frame.width < frame.height:
            raise RuntimeError('Expected the observed landscape game login UI')
        stage = 'login-page-check'
        try:
            # A deployment may leave the native reconnect modal above town.
            # Require the empty login dialog before reading or entering secrets.
            for login_attempt in range(4):
                login_labels = self.ocr()
                if (any('游戏登录' in row['text'] for row in login_labels)
                        and any('账号' in row['text'] and '请输入' in row['text'] and .2 < row['y'] < .4
                                for row in login_labels)
                        and any('请输入密码' in row['text'] and .3 < row['y'] < .5
                                for row in login_labels)):
                    break
                if login_attempt == 3:
                    raise ValueError('Empty native login dialog has not been observed')
                # Unity can expose the empty panel before its labels render.
                # Observe the same dialog without any input or credential read.
                time.sleep(1)
            stage = 'saved-account-check'
            saved = json.loads(account.read_text())
            if any(not isinstance(saved.get(key), str) or not saved[key]
                   for key in ('uid', 'password')):
                raise ValueError('Invalid saved account')
            expected_id = saved.get('player_id')
            if expected_id is not None and (type(expected_id) is not int or expected_id <= 0):
                raise ValueError('Invalid saved player reference')
            if saved.get('initialized') and expected_id is None:
                raise ValueError('Initialized account lacks its saved player reference')
            # Import the read-only local DB adapter only for saved login. The
            # container retains DB credentials; query only this saved UID and
            # return a session digest, never the session or password columns.
            if str(ROOT) not in sys.path:
                sys.path.insert(0, str(ROOT))
            from tools.demo_accounts import sql
            query = ('SELECT id,SHA2(session,256) FROM profiles WHERE uid='
                     f'CONVERT(0x{saved["uid"].encode().hex()} USING utf8mb4) '
                     'AND player IS NOT NULL;')

            def fingerprint():
                rows = sql(query).splitlines()
                if len(rows) != 1:
                    raise ValueError('Saved target profile is unavailable')
                player_id, digest = rows[0].split('\t')
                player_id = int(player_id)
                if player_id <= 0 or not re.fullmatch(r'[0-9a-fA-F]{64}', digest):
                    raise ValueError('Invalid target session fingerprint')
                if expected_id is not None and player_id != expected_id:
                    raise ValueError('Saved player reference does not match target')
                return player_id, digest.lower()

            before = fingerprint()
            stage = 'credential-entry'
            # Wait for Unity's Android editor before sending each field. A short
            # focus delay previously dropped the first UID character. Enter each
            # field once, then allow the IME and Unity to commit before submitting.
            for x, y, key in [(0.5286, 0.3151, 'uid'), (0.5268, 0.4095, 'password')]:
                self.run('shell', 'input', 'tap', round(x * frame.width), round(y * frame.height))
                time.sleep(1.5)
                self.run('shell', 'input', 'text', saved[key])
                time.sleep(0.5)
                self.run('shell', 'input', 'keyevent', 'KEYCODE_BACK')
                time.sleep(0.5)
            # The keyboard can move the horizontal inset; this point stays inside
            # the observed login button in both landscape viewport positions.
            self.run('shell', 'input', 'tap', round(0.56 * frame.width),
                     round(0.5556 * frame.height))
            time.sleep(1.2)
            # The original login dialog only requests the server list. Actual
            # CSUserLogin occurs after Enter Game; verify this selector before
            # that one native action, then bind the resulting session to the UID.
            # The server list can arrive after the first screenshot under concurrent builds.
            # Observe the same submission within a bounded window; never resend credentials.
            selector_deadline = time.monotonic() + 12
            stage = 'server-selector-check'
            for selector_attempt in range(4):
                selector = self.ocr('当前区')
                if any('当前区' in entry['text'] for entry in selector):
                    break
                if selector_attempt == 3 or time.monotonic() >= selector_deadline:
                    raise ValueError('Server selector has not loaded')
                time.sleep(1)
            self.tap_text('进入游戏')
            time.sleep(1.2)
            stage = 'target-session-check'
            # Observe this single native authentication. A typo or wrong password
            # leaves the expected UID's session unchanged; stop without retyping
            # or creating a character. Bound polling when DB calls are slow.
            deadline = time.monotonic() + 8
            for attempt in range(6):
                after = fingerprint()
                if after[0] != before[0]:
                    raise ValueError('Target profile changed during login')
                if after[1] != before[1]:
                    role = saved.get('role')
                    return {'login': 'verified',
                            'account_role': role if role in ('starter', 'systems') else 'original'}
                if attempt == 5 or time.monotonic() >= deadline:
                    break
                time.sleep(0.8)
            raise ValueError('Target session did not rotate')
        except Exception:
            # ADB errors and DB exceptions can contain input argv or SQL. Keep
            # every saved-login failure opaque and suppress credential context.
            raise RuntimeError('Saved login stopped at ' + stage + '; target account verification failed. '
                               'Inspect the login screen before any further input') from None


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serial')
    parser.add_argument('--package', choices=(PACKAGE, MULTI_PACKAGE), default=PACKAGE)
    sub = parser.add_subparsers(dest='action', required=True)
    for action in ['status', 'prepare', 'restart', 'restore-display']:
        sub.add_parser(action)
    connect = sub.add_parser('connect-local')
    connect.add_argument('--http-port', type=int, default=8088)
    connect.add_argument('--tcp-port', type=int, default=19098)
    install = sub.add_parser('install')
    install.add_argument('--apk', default=str(ROOT / 'build/hyjs-local.apk'))
    capture = sub.add_parser('capture')
    capture.add_argument('--output', default='/tmp/hyjs-device.jpg')
    wait = sub.add_parser('wait-rendered')
    wait.add_argument('--timeout', type=float, default=40)
    tap = sub.add_parser('tap')
    tap.add_argument('x', type=float)
    tap.add_argument('y', type=float)
    swipe = sub.add_parser('swipe')
    for coordinate in ['x1', 'y1', 'x2', 'y2']:
        swipe.add_argument(coordinate, type=float)
    swipe.add_argument('--duration-ms', type=int, default=1500)
    login = sub.add_parser('login')
    login.add_argument('--account', default=str(ROOT / '.local-env/game-test.json'))
    ocr = sub.add_parser('ocr')
    ocr.add_argument('--contains', default='')
    text_tap = sub.add_parser('tap-text')
    text_tap.add_argument('text')
    text_tap.add_argument('--offset-x', type=float, default=0)
    text_tap.add_argument('--offset-y', type=float, default=0)
    args = parser.parse_args()
    device = Device(args.serial, args.package)
    if args.action == 'prepare':
        device.prepare()
        result = device.status()
    elif args.action == 'connect-local':
        result = device.connect_local(args.http_port, args.tcp_port)
    elif args.action == 'install':
        result = device.install(args.apk)
    elif args.action == 'capture':
        result = device.capture(args.output)
    elif args.action == 'wait-rendered':
        result = device.wait_rendered(args.timeout)
    elif args.action == 'restart':
        device.restart()
        result = device.wait_rendered(40)
    elif args.action == 'tap':
        device.require_game_surface_accessible()
        if not device.game_is_foreground():
            raise RuntimeError('Coordinate actions are scoped to the foreground demo game')
        if not (0 <= args.x <= 1 and 0 <= args.y <= 1):
            parser.error('Tap coordinates must be fractions between 0 and 1')
        frame = device.frame()
        x = min(frame.width - 1, round(args.x * frame.width))
        y = min(frame.height - 1, round(args.y * frame.height))
        device.run('shell', 'input', 'tap', x, y)
        result = {'tap': [x, y]}
    elif args.action == 'swipe':
        device.require_game_surface_accessible()
        if not device.game_is_foreground():
            raise RuntimeError('Coordinate actions are scoped to the foreground demo game')
        fractions = [args.x1, args.y1, args.x2, args.y2]
        if not all(0 <= value <= 1 for value in fractions) or not 1 <= args.duration_ms <= 10000:
            parser.error('Swipe coordinates must be fractions; duration must be 1..10000 ms')
        frame = device.frame()
        pixels = [min((frame.width if i % 2 == 0 else frame.height) - 1,
                      round(value * (frame.width if i % 2 == 0 else frame.height)))
                  for i, value in enumerate(fractions)]
        device.run('shell', 'input', 'swipe', *pixels, args.duration_ms)
        result = {'swipe': pixels, 'duration_ms': args.duration_ms}
    elif args.action == 'restore-display':
        device.restore_display()
        result = {'display': 'restored'}
    elif args.action == 'login':
        result = device.login_saved(args.account)
    elif args.action == 'ocr':
        result = device.ocr(args.contains)
    elif args.action == 'tap-text':
        result = device.tap_text(args.text, args.offset_x, args.offset_y)
    else:
        result = device.status()
    print(json.dumps(result, ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except RuntimeError as error:
        print(json.dumps({'error': str(error)}, ensure_ascii=False), file=sys.stderr)
        sys.exit(1)
