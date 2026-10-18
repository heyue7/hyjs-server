"""Map the authorized Android game to the verified local server's actual ports."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[1]
ADB = Path.home() / 'Library/Android/sdk/platform-tools/adb'


def run(*args):
    return subprocess.run(['rtk', 'proxy', *map(str,args)],check=True,capture_output=True,text=True,timeout=20).stdout.strip()


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--serial')
    args=parser.parse_args()
    devices=[row.split()[0] for row in run(ADB,'devices').splitlines()[1:]
             if len(row.split())==2 and row.split()[1]=='device' and ':' not in row.split()[0]]
    if args.serial:
        if args.serial not in devices: raise RuntimeError('The requested authorized USB device is unavailable')
        serial=args.serial
    elif len(devices)==1:
        serial=devices[0]
    else:
        raise RuntimeError('Specify one authorized USB device')
    pid=int((ROOT/'server/data/go-server.pid').read_text().strip())
    command=run('ps','-p',pid,'-o','comm=')
    if Path(command).resolve()!=ROOT/'build/hyjs-server':
        raise RuntimeError('Saved process does not belong to this server')
    listeners=run('lsof','-nP','-a','-p',pid,'-iTCP','-sTCP:LISTEN','-Fpn')
    ports={int(match[1]) for row in listeners.splitlines() if (match:=re.fullmatch(r'n.*:(\d+)',row))}
    if 8088 not in ports or len(ports)!=2:
        raise RuntimeError('Inspect the actual local HTTP/TCP listeners')
    tcp=next(port for port in ports if port!=8088)
    # Keep the previous 9000 connection usable across the original client's
    # reconnect, and expose the newly advertised port for subsequent logins.
    mappings={8088:8088,9000:tcp,tcp:tcp}
    for phone,host in mappings.items(): run(ADB,'-s',serial,'reverse',f'tcp:{phone}',f'tcp:{host}')
    actual=run(ADB,'-s',serial,'reverse','--list').splitlines()
    for phone,host in mappings.items():
        if not any(row.split()[-2:]==[f'tcp:{phone}',f'tcp:{host}'] for row in actual):
            raise RuntimeError('Android USB reverse verification failed')
    with (ROOT/'build/hyjs-server').open('rb') as stream: digest=hashlib.file_digest(stream,'sha256').hexdigest()
    print(json.dumps({'serial':serial,'httpPort':8088,'tcpPort':tcp,'mappings':mappings,'binarySHA256':digest}))


if __name__=='__main__': main()
