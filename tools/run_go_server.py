"""Start the compiled Go server with local MySQL credentials kept out of argv."""
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(root))
from tools.shared_mysql import environment

# This is the local launcher. Remote and deployment templates keep their own
# connection contract; local runs always use the allocated project account.
os.environ.update(environment())
# Local admin uses a private bootstrap file and compiled static assets.
admin_bootstrap = root / '.local-env/admin-bootstrap.json'
admin_assets = root / 'admin-web/dist'
if admin_bootstrap.is_file():
    os.environ.setdefault('HYJS_ADMIN_BOOTSTRAP_FILE', str(admin_bootstrap))
if (admin_assets / 'index.html').is_file():
    os.environ.setdefault('HYJS_ADMIN_ASSETS', str(admin_assets))
os.chdir(root / 'hyjs-server')
binary = root / 'build/hyjs-server'
# 后台进程独立于本次聊天的终端，凭据只通过环境传入。
if '--daemon' in sys.argv[1:]:
    arguments = [arg for arg in sys.argv[1:] if arg != '--daemon']
    state = root / 'server/data'
    state.mkdir(parents=True, exist_ok=True)
    output = state / 'go-server.console.log'
    output.touch(mode=0o600, exist_ok=True)
    os.chmod(output, 0o600)
    with output.open('wb') as log:
        process = subprocess.Popen([str(binary), *arguments], stdin=subprocess.DEVNULL,
                                   stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    # 联合域首次建表和规则初始化需要更长时间；仅等待同一进程，不重复启动。
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if process.poll() is not None:
            sys.exit('Go server start failed; see server/data/go-server.console.log')
        if b'HYJS Go ready ' in output.read_bytes():
            pid = state / 'go-server.pid'
            pid.touch(mode=0o600, exist_ok=True)
            os.chmod(pid, 0o600)
            pid.write_text(str(process.pid) + '\n')
            print('HYJS Go ready in background; pid saved to server/data/go-server.pid')
            sys.exit(0)
        time.sleep(0.1)
    process.terminate()
    process.wait(timeout=5)
    sys.exit('Go server start timed out; see server/data/go-server.console.log')
os.execv(str(binary), [str(binary), *sys.argv[1:]])
