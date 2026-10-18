// Explicit local PCM adapter. Original GCloud authentication/codec/ASR is not emulated.
// The server still defaults off. Do not log session bytes, openId or credentials.
using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Security.Cryptography;
using System.Text;
using LitJson;
using UnityEngine;

public sealed class LocalVoiceBridge : MonoBehaviour
{
    public static bool Enabled = false;
    private static string origin;
    private static LocalVoiceBridge instance;
    private AudioSource speaker;
    private AudioClip recording;
    private AudioClip playing;
    private long recordPlayer;
    private byte[] recordSession;
    private string recordScope;
    private int recordType;
    private float recordStarted, lastRecordStopped = -2;
    private int recordGeneration, playGeneration;
    private bool recordingReady;
    private Delegate recordCallback;
    private object recordData;
    private Queue<Playback> queue = new Queue<Playback>();
    private Playback current;
    private sealed class Playback { public string name; public Delegate callback; public object data; public long player; public byte[] session; }

    // Only an explicit APK build integration can enable this adapter and bind its local server origin.
    public static void Configure(string baseURI, bool enabled)
    {
        Uri uri;
        if (!Uri.TryCreate(baseURI, UriKind.Absolute, out uri) || (uri.Scheme != "http" && uri.Scheme != "https") || !String.IsNullOrEmpty(uri.UserInfo) || !String.IsNullOrEmpty(uri.Query) || !String.IsNullOrEmpty(uri.Fragment)) throw new ArgumentException("Invalid local voice origin");
        origin = uri.GetLeftPart(UriPartial.Authority);
        Enabled = enabled;
        if (!enabled && instance != null) instance.CancelAll();
    }
    private static LocalVoiceBridge Host()
    {
        if (instance == null) { GameObject obj = new GameObject("LocalOriginalVoicePCM"); UnityEngine.Object.DontDestroyOnLoad(obj); instance = obj.AddComponent<LocalVoiceBridge>(); instance.speaker = obj.AddComponent<AudioSource>(); }
        return instance;
    }
    private static object Get(object obj, string field) { if (obj == null) return null; FieldInfo f = obj.GetType().GetField(field, BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.Instance); return f == null ? null : f.GetValue(obj); }
    private static void Set(object obj, string field, object value) { if (obj == null) return; FieldInfo f = obj.GetType().GetField(field, BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.Instance); if (f != null) f.SetValue(obj,value); }
    private static long Player() { return StaticData.playerData == null ? 0 : StaticData.playerData.playerId; }
    private static byte[] Session() { return StaticData.sessionKey == null ? new byte[0] : (byte[])StaticData.sessionKey.Clone(); }
    private static bool Same(long player, byte[] session) { byte[] now = StaticData.sessionKey; if (player <= 0 || Player() != player || now == null || now.Length != 16 || session == null || session.Length != 16) return false; int diff=0; for(int i=0;i<16;i++) diff |= now[i]^session[i]; return diff==0; }
    private static Dictionary<string,string> Headers(long player, byte[] session)
    { Dictionary<string,string> h=new Dictionary<string,string>(); h["X-Hyjs-Player-ID"]=player.ToString(System.Globalization.CultureInfo.InvariantCulture);h["X-Hyjs-Session-Key"]=Convert.ToBase64String(session);h["Cache-Control"]="no-store";return h; }
    private static void Callback(Delegate callback, object data, bool ok) { if(callback==null)return;try { callback.DynamicInvoke(data,ok); } catch { } }
    // Original PlayEffect's bool selects downloading; isStop separately ends playback.
    // Recording callbacks retain their original success/failure boolean.
    private static void PlaybackStatus(Delegate callback, object data, bool downloading, bool stopped)
    { Set(data,"isStop",stopped);Callback(callback,data,downloading); }
    private static bool Opaque(string name) { if(name==null||name.Length!=46||!name.StartsWith("hyjsvoice_")||!name.EndsWith(".wav"))return false;for(int i=10;i<42;i++){char c=name[i];if(!(c>='0'&&c<='9'||c>='a'&&c<='f'))return false;}return true; }
    private static string SHA(byte[] bytes) { using(SHA256 sha=SHA256.Create()) { byte[] sum=sha.ComputeHash(bytes);StringBuilder s=new StringBuilder();foreach(byte b in sum)s.Append(b.ToString("x2"));return s.ToString(); } }
    private static JsonData Data(WWW request) { if(!String.IsNullOrEmpty(request.error))throw new InvalidOperationException("Local voice HTTP failed");JsonData obj=JsonMapper.ToObject(request.text);if((int)obj["code"]!=0)throw new InvalidOperationException("Local voice permission refused");return obj["data"]; }
    private static int Int(JsonData data,string key) { return Convert.ToInt32(data[key].ToString(),System.Globalization.CultureInfo.InvariantCulture); }
    private static bool Bool(JsonData data,string key) { return (bool)data[key]; }
    private static bool Active() { return Enabled && origin != null && Player()>0 && StaticData.sessionKey!=null && StaticData.sessionKey.Length==16; }

    public static void Init(object plugin, string ignoredOpenId) { if(Active())Host(); }
    public static void StartRecord(object plugin)
    {
        if(!Active())return;LocalVoiceBridge h=Host();h.CancelRecord();h.StopPlayback();if(Time.realtimeSinceStartup-h.lastRecordStopped<1)return;
        h.recordPlayer=Player();h.recordSession=Session();h.recordType=Convert.ToInt32(Get(plugin,"talkType"));int generation=++h.recordGeneration;h.StartCoroutine(h.BeginRecord(generation));
    }
    private IEnumerator BeginRecord(int generation)
    {
        Dictionary<string,string> headers=Headers(recordPlayer,recordSession);
        using(WWW request=new WWW(origin+"/voice/config?chatType="+recordType,null,headers)) {
            float deadline=Time.realtimeSinceStartup+10;while(!request.isDone&&Time.realtimeSinceStartup<deadline&&generation==recordGeneration&&Same(recordPlayer,recordSession))yield return null;
            if(!request.isDone||generation!=recordGeneration||!Same(recordPlayer,recordSession))yield break;
            bool allowed=false;try {JsonData data=Data(request);allowed=Bool(data,"enabled")&&Bool(data,"canRecord")&&Int(data,"adapter")==1&&Int(data,"maxSeconds")==30&&Int(data,"recordingGapSeconds")==1;recordScope=(string)data["scope"];}catch { }
            if(!allowed||String.IsNullOrEmpty(recordScope)||Microphone.devices.Length==0)yield break;
            recording=Microphone.Start(null,false,30,16000);recordStarted=Time.realtimeSinceStartup;recordingReady=recording!=null;
        }
    }
    public static void StopRecord(object plugin, object callback, object data, bool accepted)
    {
        LocalVoiceBridge h=Host();h.recordCallback=callback as Delegate;h.recordData=data;h.lastRecordStopped=Time.realtimeSinceStartup;
        int type=Convert.ToInt32(Get(data,"type"));
        if(!Active()||!accepted||!h.recordingReady||type!=h.recordType||!Same(h.recordPlayer,h.recordSession)) { h.FailRecord();return; }
        int samples=Microphone.GetPosition(null);if(Time.realtimeSinceStartup-h.recordStarted>=30)samples=h.recording.samples;
        Microphone.End(null);h.recordingReady=false;byte[] bytes=null;
        try { if(h.recording.frequency!=16000||h.recording.channels!=1||samples<=0||samples>480000)throw new InvalidOperationException();float[] values=new float[samples];if(!h.recording.GetData(values,0))throw new InvalidOperationException();bytes=Encode(values); }catch { }
        UnityEngine.Object.Destroy(h.recording);h.recording=null;
        if(bytes==null){h.FailRecord();return;}int generation=h.recordGeneration;h.StartCoroutine(h.Upload(generation,bytes));
    }
    private IEnumerator Upload(int generation,byte[] bytes)
    {
        long player=recordPlayer;byte[] session=recordSession;string scope=recordScope;int type=recordType;Delegate callback=recordCallback;object data=recordData;
        Dictionary<string,string> headers=Headers(player,session);headers["Content-Type"]="audio/wav";headers["X-Hyjs-Chat-Type"]=type.ToString();headers["X-Hyjs-Voice-Scope"]=scope;headers["X-Hyjs-Request-ID"]=Guid.NewGuid().ToString("N");
        string name=null;bool valid=false;
        using(WWW request=new WWW(origin+"/voice/upload",bytes,headers)) {
            float deadline=Time.realtimeSinceStartup+60;while(!request.isDone&&Time.realtimeSinceStartup<deadline)yield return null;
            if(request.isDone)try {JsonData result=Data(request);name=(string)result["fileName"];int duration=(bytes.Length-44)*1000/32000;valid=Opaque(name)&&Int(result,"fileSize")==bytes.Length&&Int(result,"durationMillis")==duration&&Int(result,"audioTime")==duration/1000&&(string)result["sha256"]==SHA(bytes);}catch { }
        }
        if(generation!=recordGeneration||!Same(player,session)||!Active()) { if(valid&&Same(player,session))StartCoroutine(DeleteOwn(player,session,name));yield break; }
        recordCallback=null;recordData=null;
        if(valid){Set(data,"fileId",name);Set(data,"timeNap",(bytes.Length-44)/32000);Set(data,"fileSize",bytes.Length);Set(data,"content","");}
        Callback(callback,data,valid);
    }
    private IEnumerator DeleteOwn(long player,byte[] session,string name)
    { if(!Same(player,session)||!Opaque(name))yield break;Dictionary<string,string> headers=Headers(player,session);headers["X-Hyjs-File-Name"]=name;using(WWW request=new WWW(origin+"/voice/delete",new byte[0],headers)){float deadline=Time.realtimeSinceStartup+10;while(!request.isDone&&Time.realtimeSinceStartup<deadline)yield return null;} }
    public static void ClearRecord(object plugin) { if(instance!=null)instance.CancelRecord(); }
    private void FailRecord() { Delegate callback=recordCallback;object data=recordData;CancelRecord();Callback(callback,data,false); }
    private void CancelRecord() { recordGeneration++;recordCallback=null;recordData=null;recordingReady=false;Microphone.End(null);if(recording!=null)UnityEngine.Object.Destroy(recording);recording=null; }

    public static void PlayRecord(object plugin,string name,object callback,int ignoredDuration,object data)
    { if(!Active()){PlaybackStatus(callback as Delegate,data,false,true);return;}Host().Enqueue(name,callback as Delegate,data,true); }
    public static void Download(object plugin,string name,int type,bool fromPage,int ignoredDuration,object data)
    { Delegate callback=fromPage?Get(plugin,"playDelegate") as Delegate:null;if(!Active()){PlaybackStatus(callback,data,false,true);return;}bool allowed=fromPage;try {allowed=allowed||Chat.ChatManager.Instance.GetAudioPlay((Chat.ChatType)type);}catch { }if(allowed)Host().Enqueue(name,callback,data,fromPage); }
    private void Enqueue(string name,Delegate callback,object data,bool manual)
    {
        if(!Opaque(name)){PlaybackStatus(callback,data,false,true);return;}if(manual)StopPlayback();while(queue.Count>=10){Playback old=queue.Dequeue();PlaybackStatus(old.callback,old.data,false,true);}Playback item=new Playback();item.name=name;item.callback=callback;item.data=data;item.player=Player();item.session=Session();queue.Enqueue(item);
        PlaybackStatus(callback,data,true,false);
        if(current==null)StartCoroutine(PlayNext(playGeneration));
    }
    private IEnumerator PlayNext(int generation)
    {
        while(queue.Count>0&&generation==playGeneration) {
            current=queue.Dequeue();Playback item=current;byte[] raw=null;
            if(Same(item.player,item.session)&&Active()) {
                // Fetch at the actual playback boundary; there is no disk-cache or earlier-ticket bypass.
                Dictionary<string,string> headers=Headers(item.player,item.session);headers["X-Hyjs-File-Name"]=item.name;
                using(WWW request=new WWW(origin+"/voice/download",new byte[0],headers)) {float deadline=Time.realtimeSinceStartup+60;while(!request.isDone&&Time.realtimeSinceStartup<deadline&&generation==playGeneration&&Same(item.player,item.session))yield return null;if(request.isDone&&String.IsNullOrEmpty(request.error))raw=request.bytes;}
            }
            AudioClip clip=null;if(raw!=null)try {clip=Decode(raw);}catch { }
            if(generation!=playGeneration){if(clip!=null)UnityEngine.Object.Destroy(clip);yield break;}
            if(clip!=null&&Same(item.player,item.session)&&Active()) {playing=clip;PlaybackStatus(item.callback,item.data,false,false);speaker.clip=clip;speaker.Play();while(speaker.isPlaying&&generation==playGeneration&&Same(item.player,item.session))yield return null;speaker.Stop();UnityEngine.Object.Destroy(clip);playing=null;}
            if(generation!=playGeneration)yield break;PlaybackStatus(item.callback,item.data,false,true);current=null;
        }
    }
    public static void Play(object plugin,object data) { string name=Get(data,"fileId") as string;Host().Enqueue(name,Get(data,"playDelegate") as Delegate,Get(data,"data"),false); }
    public static void StopPlay(object plugin) { if(instance!=null)instance.StopPlayback(); }
    // Original DeleFile is local history eviction; it never authorizes remote deletion of another owner's object.
    public static void Evict(object plugin,string ignoredPath) { }
    public static void Tick(object plugin) { }
    private void StopPlayback() { playGeneration++;if(speaker!=null)speaker.Stop();if(playing!=null)UnityEngine.Object.Destroy(playing);playing=null;if(current!=null){PlaybackStatus(current.callback,current.data,false,true);}current=null;while(queue.Count>0){Playback item=queue.Dequeue();PlaybackStatus(item.callback,item.data,false,true);} }
    private void CancelAll(){CancelRecord();StopPlayback();}
    private void OnApplicationPause(bool paused){if(paused)CancelAll();}
    private void Update(){if(recordSession!=null&&!Same(recordPlayer,recordSession))CancelRecord();if(current!=null&&!Same(current.player,current.session))StopPlayback();}
    private void OnDestroy(){CancelAll();if(instance==this)instance=null;}

    public static byte[] Encode(float[] values)
    { if(values==null||values.Length==0||values.Length>480000)throw new ArgumentException();byte[] raw=new byte[44+values.Length*2];WriteText(raw,0,"RIFF");Write32(raw,4,raw.Length-8);WriteText(raw,8,"WAVEfmt ");Write32(raw,16,16);Write16(raw,20,1);Write16(raw,22,1);Write32(raw,24,16000);Write32(raw,28,32000);Write16(raw,32,2);Write16(raw,34,16);WriteText(raw,36,"data");Write32(raw,40,values.Length*2);for(int i=0;i<values.Length;i++){float value=values[i];if(Single.IsNaN(value)||Single.IsInfinity(value))throw new ArgumentException();value=Math.Max(-1,Math.Min(1,value));short sample=(short)Math.Round(value*(value<0?32768:32767));Write16(raw,44+i*2,sample);}return raw; }
    public static AudioClip Decode(byte[] raw)
    { if(raw==null||raw.Length<76||raw.Length>960044||Encoding.ASCII.GetString(raw,0,4)!="RIFF"||Read32(raw,4)!=raw.Length-8||Encoding.ASCII.GetString(raw,8,8)!="WAVEfmt "||Read32(raw,16)!=16||Read16(raw,20)!=1||Read16(raw,22)!=1||Read32(raw,24)!=16000||Read32(raw,28)!=32000||Read16(raw,32)!=2||Read16(raw,34)!=16||Encoding.ASCII.GetString(raw,36,4)!="data"||Read32(raw,40)!=raw.Length-44||(raw.Length-44)%2!=0)throw new ArgumentException();int n=(raw.Length-44)/2;float[] values=new float[n];for(int i=0;i<n;i++)values[i]=(short)Read16(raw,44+i*2)/32768f;AudioClip clip=AudioClip.Create("AuthenticatedLocalVoice",n,1,16000,false);if(!clip.SetData(values,0)){UnityEngine.Object.Destroy(clip);throw new InvalidOperationException();}return clip; }
    private static void WriteText(byte[] raw,int offset,string text){byte[] value=Encoding.ASCII.GetBytes(text);Buffer.BlockCopy(value,0,raw,offset,value.Length);}
    private static void Write16(byte[] raw,int offset,int value){raw[offset]=(byte)value;raw[offset+1]=(byte)(value>>8);}
    private static void Write32(byte[] raw,int offset,int value){for(int i=0;i<4;i++)raw[offset+i]=(byte)(value>>(i*8));}
    private static int Read16(byte[] raw,int offset){return raw[offset]|raw[offset+1]<<8;}
    private static int Read32(byte[] raw,int offset){return raw[offset]|raw[offset+1]<<8|raw[offset+2]<<16|raw[offset+3]<<24;}
}
