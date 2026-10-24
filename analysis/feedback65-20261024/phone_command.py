import sys,time,json
from pathlib import Path
ROOT=Path(__file__).resolve().parents[2];sys.path.insert(0,str(ROOT/"tools"))
from device_demo import Device
d=Device("10AF7Q0KSJ003GK");cmd=" ".join(sys.argv[1:]);assert cmd and len(cmd)<=128
p=ROOT/".local-env/feedback65/phone/probe-command.tmp";p.write_text(cmd);tmp="/data/local/tmp/hyjs65-command"
d.run("push",p,tmp);d.run("shell","run-as",d.package,"cp",tmp,"files/hyjs65-qa/command.tmp");d.run("shell","run-as",d.package,"mv","files/hyjs65-qa/command.tmp","files/hyjs65-qa/command.txt");d.run("shell","rm",tmp)
for _ in range(30):
 time.sleep(.2)
 if "command.txt" not in d.text("shell","run-as",d.package,"ls","files/hyjs65-qa").splitlines():break
else:raise RuntimeError("Probe command not consumed")
rows=d.text("shell","run-as",d.package,"cat","files/hyjs65-qa/observations.jsonl").splitlines(); events=[json.loads(l) for l in rows if "command-" in l];print(events[-1] if events else "no command event")
time.sleep(.8);print(d.capture(ROOT/"analysis/feedback65-20261024/phone-current.png"))
