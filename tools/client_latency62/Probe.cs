using System.Net;
using System.Diagnostics;
using System.Text.Json;
using HYJS.Latency62;
static void Check(bool ok,string text){if(!ok)throw new Exception(text);}
foreach(float age in new[]{0f,.10f,.249f})Check(Policy.Compact(age)==0 && Policy.Full(age)==0,"fast visual flash");
Check(Math.Abs(Policy.Compact(.30f)-.5f)<.001f,"compact fade");
Check(Math.Abs(Policy.Compact(.35f)-1)<.001f && Policy.Full(1.49f)==0,"compact range");
Check(Math.Abs(Policy.Full(1.55f)-.5f)<.001f,"full fade");
Check(Policy.Full(1.61f)==1 && Policy.Compact(1.61f)==0,"full range");
var request=(HttpWebRequest)WebRequest.Create("http://127.0.0.1:9876/original?x=1");
request.Timeout=15000;request.ReadWriteTimeout=15000;request.Proxy=null;
var uri=request.RequestUri;
var same=NetworkParameters.Configure(request);
Check(ReferenceEquals(same,request) && request.RequestUri==uri,"request identity/uri");
Check(request.KeepAlive && !request.ServicePoint.Expect100Continue && !request.ServicePoint.UseNagleAlgorithm,"transport flags");
Check(request.Timeout==15000 && request.ReadWriteTimeout==15000 && request.Proxy==null,"original request contracts");
var watch=Stopwatch.StartNew();int callback=0;
// The caller handles a real completion immediately, even during visual stages.
foreach(float age in new[]{.1f,.3f,1.55f}) { _=Policy.Compact(age);_=Policy.Full(age);callback++; }
Check(callback==3 && watch.ElapsedMilliseconds<100,"policy blocks caller");
File.WriteAllText(args[0],JsonSerializer.Serialize(new {policyBoundariesPassed=true,realHttpWebRequestFlagsPassed=true,requestIdentityURIProxyAndTimeoutPreserved=true,policyHasNoCompletionWait=true,runtime="net8 host probe; original Unity/Mono device behavior unverified",networkSent=false},new JsonSerializerOptions{WriteIndented=true}));
