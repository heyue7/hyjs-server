"""Run the exact notice bridge's plain-content parser; does not execute Unity UI."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PROGRAM = r'''
using System;
using HYJS.Notices;
static class Program {
 static int checks;
 static void Check(bool value,string name){if(!value)throw new Exception(name);checks++;}
 static bool Reject(string html){try{OriginalNotices.Parse(html);return false;}catch(FormatException){return true;}}
 static void Main(){
  var empty=OriginalNotices.Parse("<html><p class=\"empty\">暂无公告</p></html>");
  Check(empty.Count==0,"empty server response");
  var list=OriginalNotices.Parse("<html><article><h2>第一条 &amp; 更新</h2><p>第一行\n第二行&lt;script&gt;alert(1)&lt;/script&gt;</p></article><article><h2>第二条</h2><p>奖励&#34;说明&#34; &#x1f338; &amp;lt;raw&amp;gt;</p></article></html>");
  Check(list.Count==2,"two items retained");
  Check(list[0].Title=="第一条 & 更新"&&list[1].Title=="第二条","original ordering and title escapes");
  Check(list[0].Content=="第一行\n第二行<script>alert(1)</script>","plain escaped text and line breaks");
  Check(list[1].Content=="奖励\"说明\" 🌸 &lt;raw&gt;","numeric entities and exactly one decoding pass");
  Check(OriginalNotices.Decode("A &unknown; B &#xD800; C & no terminator")=="A &unknown; B &#xD800; C & no terminator","unknown and invalid entities preserved");
  Check(Reject("404 not found"),"unknown response rejected");
  Check(Reject("<article><h2>未结束</h2><p>内容</p>"),"truncated article rejected");
  Check(Reject("<article><h2>缺正文</h2></article>"),"missing content rejected");
  string longText=new string('文',100000)+"\n最后一行";
  var longList=OriginalNotices.Parse("<article><h2>长公告</h2><p>"+longText+"</p></article>");
  Check(longList.Count==1&&longList[0].Content==longText,"long content retains all text");
  Check(OriginalNotices.Decode("&lt;b&gt;[FF0000]标题[-]&lt;/b&gt;")=="<b>[FF0000]标题[-]</b>","styling syntax remains literal content");
  Console.WriteLine("PASS "+checks+" exact-binary announcement content assertions");
 }
}
'''


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--bridge', type=Path, required=True)
    p.add_argument('--output', type=Path, required=True)
    args = p.parse_args()
    dotnet = ROOT / '.tools/native-batch/dotnet/dotnet'
    with tempfile.TemporaryDirectory(prefix='hyjs-notice-content-') as temporary:
        work = Path(temporary)
        shutil.copy2(args.bridge, work / 'SoulFlowerNotices.dll')
        (work / 'Program.cs').write_text(PROGRAM)
        project = work / 'Probe.csproj'
        project.write_text('<Project Sdk="Microsoft.NET.Sdk"><PropertyGroup><OutputType>Exe</OutputType><TargetFramework>net8.0</TargetFramework></PropertyGroup><ItemGroup><Reference Include="SoulFlowerNotices"><HintPath>SoulFlowerNotices.dll</HintPath></Reference></ItemGroup></Project>')
        result = subprocess.run(['rtk', 'proxy', str(dotnet), 'build', str(project), '--nologo', '--verbosity', 'quiet', '-p:NuGetAudit=false'], capture_output=True, text=True, timeout=90)
        if result.returncode:
            args.output.with_suffix('.compile.log').write_text(result.stdout + result.stderr)
            raise RuntimeError('Content probe build failed; inspect log')
        run = subprocess.run(['rtk', 'proxy', str(dotnet), str(work / 'bin/Debug/net8.0/Probe.dll')], capture_output=True, text=True, timeout=15)
        report = dict(mode='exact-notice-bridge-binary-plain-parser', status='PASS' if run.returncode == 0 else 'FAIL', bridgeSHA256=hashlib.sha256(args.bridge.read_bytes()).hexdigest(), output=run.stdout + run.stderr, unityNativeOrRenderingExecuted=False, deviceExecuted=False)
        args.output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        print(json.dumps(report))
        assert run.returncode == 0


if __name__ == '__main__': main()
