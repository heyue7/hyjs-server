using System;
using System.Collections;
using System.Collections.Generic;
using System.Reflection;
using System.Text;
using LitJson;
using UnityEngine;
namespace HYJS.Sprout73 {
public sealed class Runtime : MonoBehaviour {
 static Hub hub; UIName3D label; long player; GameObject icon; Mesh mesh; Renderer iconRenderer; Bounds previous; bool geometry;
 static object Get(object o,string n) { for(Type t=o.GetType();t!=null;t=t.BaseType) { FieldInfo f=t.GetField(n,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.DeclaredOnly); if(f!=null)return f.GetValue(o); } return null; }
 static readonly int[] experience = new int[]{0,11,22,33,44,55,66,77,88,99,110,121,132,144,155,166,178,189,201,212,224,236,248,260,272,570,595,621,648,675,702,730,759,788,819,849,881,914,947,982,1017,1053,1091,1130,1170,1211,1254,1298,1344,1391,1440,1490,1542,1596,1652,1710,1770,1832,1896,1962,2031,2102,2176,2253,2332,2413,2498,2586,2676,2770,2867,2968,3071,3179,3289,3404,3522,3645,3771,3902,4036,4176,4319,4467,4620,4778,4940,5108,5280,5458,5642,5831,6025,6226,6432,6644,6862,7087,7318,7556,7800};
 public static void Experience(object value){Dict.DataModel.TableCopyExp row=value as Dict.DataModel.TableCopyExp;if(row!=null && row.level>=1 && row.level<experience.Length)row.exp=experience[row.level];}
 public static void Attach(object value) { try { object own=Get(value,"owner"); Component actor=own as Component; if(actor==null)return; int kind=Convert.ToInt32(Get(own,"type")); if(kind!=1 && kind!=2)return; long id; if(!Int64.TryParse(actor.name,out id)||id<=0)return; UIName3D name=Get(value,"label") as UIName3D; if(name==null)return; Runtime r=name.GetComponent<Runtime>(); if(r==null)r=name.gameObject.AddComponent<Runtime>(); r.Clear(); r.label=name;r.player=id; if(hub==null){GameObject go=new GameObject("HYJSSproutStatus");UnityEngine.Object.DontDestroyOnLoad(go);hub=go.AddComponent<Hub>();}hub.Add(r); } catch(Exception) {} }
 public static void Detach(object value) { try { UIName3D name=Get(value,"label") as UIName3D; if(name!=null){Runtime r=name.GetComponent<Runtime>();if(r!=null){r.Clear();}} }catch(Exception){} }
 void Clear(){if(hub!=null)hub.Remove(this);if(iconRenderer!=null && iconRenderer.sharedMaterial!=null)UnityEngine.Object.Destroy(iconRenderer.sharedMaterial);if(icon!=null)UnityEngine.Object.Destroy(icon);if(mesh!=null)UnityEngine.Object.Destroy(mesh);icon=null;mesh=null;iconRenderer=null;geometry=false;player=0;}
 void OnDestroy(){Clear();}
 public long Player {get{return player;}}
 public bool Visible {get{return player>0 && label!=null && label.gameObject.activeInHierarchy;}}
 // TextMesh owns its generated geometry internally; the original name prefab has no MeshFilter.
 // Recover its local planar size from the renderer's world bounds and the text transform.
 static bool TextBounds(TextMesh text,Renderer renderer,out Bounds bounds){
  bounds=new Bounds();Bounds world=renderer.bounds;Vector3 x=text.transform.TransformVector(Vector3.right),y=text.transform.TransformVector(Vector3.up);
  x=new Vector3(Mathf.Abs(x.x),Mathf.Abs(x.y),Mathf.Abs(x.z));y=new Vector3(Mathf.Abs(y.x),Mathf.Abs(y.y),Mathf.Abs(y.z));
  float xx=Vector3.Dot(x,x),xy=Vector3.Dot(x,y),yy=Vector3.Dot(y,y),det=xx*yy-xy*xy;if(det<=xx*yy*0.000001f)return false;
  float xs=Vector3.Dot(x,world.size),ys=Vector3.Dot(y,world.size);float width=(xs*yy-ys*xy)/det,height=(ys*xx-xs*xy)/det;
  if(width<=0.00001f||height<=0.00001f)return false;bounds=new Bounds(text.transform.InverseTransformPoint(world.center),new Vector3(width,height,0));return true;
 }
 void LateUpdate(){try{if(label==null||hub==null||!hub.Active(player)){if(icon!=null)icon.SetActive(false);return;} TextMesh text=Get(label,"text0") as TextMesh; if(text==null||String.IsNullOrEmpty(text.text)){if(icon!=null)icon.SetActive(false);return;} Renderer native=text.GetComponent<Renderer>(); Bounds b; if(native==null||!native.enabled||!TextBounds(text,native,out b)){if(icon!=null)icon.SetActive(false);return;} float h=b.size.y;
 if(icon==null){Component title=Get(label,"title") as Component;Renderer titleRenderer=title==null?null:title.GetComponent<Renderer>();if(titleRenderer==null||titleRenderer.sharedMaterial==null)return;icon=new GameObject("HYJSSproutNameIcon");icon.layer=text.gameObject.layer;icon.transform.parent=text.transform;icon.transform.localPosition=Vector3.zero;icon.transform.localRotation=Quaternion.identity;icon.transform.localScale=Vector3.one;mesh=new Mesh();icon.AddComponent<MeshFilter>().mesh=mesh;iconRenderer=icon.AddComponent<MeshRenderer>();Material material=new Material(titleRenderer.sharedMaterial);material.mainTexture=hub.Icon;iconRenderer.material=material;iconRenderer.sortingLayerID=native.sortingLayerID;iconRenderer.sortingOrder=native.sortingOrder;}
 if(!geometry || Mathf.Abs(previous.min.x-b.min.x)>h*0.01f || Mathf.Abs(previous.center.y-b.center.y)>h*0.01f || Mathf.Abs(previous.size.y-b.size.y)>h*0.01f){previous=b;geometry=true;float w=h*hub.Icon.width/hub.Icon.height;float x=b.min.x-h*0.15f-w;float y=b.center.y-h*0.5f;float z=b.center.z;mesh.vertices=new Vector3[]{new Vector3(x,y,z),new Vector3(x+w,y,z),new Vector3(x+w,y+h,z),new Vector3(x,y+h,z)};mesh.uv=new Vector2[]{new Vector2(0,0),new Vector2(1,0),new Vector2(1,1),new Vector2(0,1)};mesh.colors=new Color[]{Color.white,Color.white,Color.white,Color.white};mesh.triangles=new int[]{0,2,1,0,3,2};mesh.RecalculateBounds();}iconRenderer.enabled=true;icon.SetActive(true);
 }catch(Exception){if(icon!=null)icon.SetActive(false);}}
}
public sealed class Hub : MonoBehaviour {
 readonly List<Runtime> entries=new List<Runtime>(); readonly Dictionary<long,long> expiry=new Dictionary<long,long>();long owner,offset;string session="",origin="",signature="";float check,next;bool busy;Texture2D texture;
 public Texture2D Icon {get{if(texture==null){texture=new Texture2D(2,2,TextureFormat.ARGB32,false);using(System.IO.Stream s=typeof(Hub).Assembly.GetManifestResourceStream("ggo_sprout.png")){byte[] b=new byte[s.Length];s.Read(b,0,b.Length);texture.LoadImage(b);}texture.filterMode=FilterMode.Bilinear;}return texture;}}
 public void Add(Runtime r){if(!entries.Contains(r))entries.Add(r);signature="";check=0;}
 public void Remove(Runtime r){entries.Remove(r);bool retained=false;foreach(Runtime entry in entries)if(entry!=null&&entry.Player==r.Player){retained=true;break;}if(!retained)expiry.Remove(r.Player);signature="";check=0;}
 static long Now(){return (DateTime.UtcNow.Ticks-621355968000000000L)/10000;}
 public bool Active(long id){long until;return owner>0&&expiry.TryGetValue(id,out until)&&until>Now()+offset;}
 bool Current(out long id,out string key,out string url){id=0;key="";url="";if(StaticData.playerData==null||StaticData.sessionKey==null||StaticData.sessionKey.Length!=16)return false;id=StaticData.playerData.playerId;StringBuilder b=new StringBuilder();foreach(byte v in StaticData.sessionKey)b.Append(v.ToString("x2"));key=b.ToString();try{url=new Uri(ServerConfig.GetServerUrl()).GetLeftPart(UriPartial.Authority);}catch{return false;}return id>0;}
 void Update(){long id;string key,url;if(!Current(out id,out key,out url)){owner=0;session="";expiry.Clear();signature="";return;}if(id!=owner||session!=key||origin!=url){owner=id;session=key;origin=url;expiry.Clear();signature="";offset=0;next=0;check=0;}if(Time.realtimeSinceStartup<check)return;check=Time.realtimeSinceStartup+1;List<long> ids=new List<long>();
 // Keep the owner in a full visible roster so a failed peer cannot hide the owner's status indefinitely.
 foreach(Runtime r in entries)if(r!=null&&r.Visible&&r.Player==owner){ids.Add(owner);break;}
 foreach(Runtime r in entries)if(r!=null&&r.Visible&&!ids.Contains(r.Player)&&ids.Count<100)ids.Add(r.Player);ids.Sort();string sig=String.Join(",",ids.ConvertAll(delegate(long p){return p.ToString();}).ToArray());if(sig!=signature){signature=sig;next=0;}if(!busy&&ids.Count>0&&Time.realtimeSinceStartup>=next){next=Time.realtimeSinceStartup+30;StartCoroutine(Fetch(ids,owner,session,origin,sig));}}
 IEnumerator Fetch(List<long> ids,long who,string key,string url,string sig){busy=true;List<long> targets=ids;
 for(int attempt=0;attempt<2;attempt++){
  WWW request=null;try{JsonData body=new JsonData();body["playerId"]=who;body["sessionHex"]=key;JsonData list=new JsonData();list.SetJsonType(JsonType.Array);foreach(long id in targets)list.Add(id);body["playerIds"]=list;Dictionary<string,string> headers=new Dictionary<string,string>();headers["Content-Type"]="application/json";request=new WWW(url+"/sprout-status",Encoding.UTF8.GetBytes(JsonMapper.ToJson(body)),headers);}catch(Exception){}
  float deadline=Time.realtimeSinceStartup+15;while(request!=null&&!request.isDone&&Time.realtimeSinceStartup<deadline)yield return null;
  bool ok=false;try{long current;string currentKey,currentUrl;if(owner!=who||session!=key||origin!=url||signature!=sig||!Current(out current,out currentKey,out currentUrl)||current!=who||currentKey!=key||currentUrl!=url)break;
   if(request!=null&&request.isDone&&String.IsNullOrEmpty(request.error)){JsonData root=JsonMapper.ToObject(request.text);if(Int32.Parse(root["code"].ToString())==0){JsonData data=root["data"];if((string)data["icon"]!="ggo_sprout")throw new Exception();long server=Int64.Parse(data["serverTime"].ToString());Dictionary<long,long> result=new Dictionary<long,long>();foreach(JsonData p in data["players"]){long target=Int64.Parse(p["playerId"].ToString());if(!targets.Contains(target)||result.ContainsKey(target))throw new Exception();result[target]=(bool)p["active"]?Int64.Parse(p["expiresAt"].ToString()):0;}offset=server-Now();foreach(long target in targets)expiry[target]=result.ContainsKey(target)?result[target]:0;ok=true;}}
  }catch(Exception){}finally{if(request!=null)request.Dispose();}
  if(ok)break;foreach(long target in targets)expiry[target]=0;
  // The existing batch contract rejects every result if one peer has left. Retry only the visible owner once.
  if(attempt==0&&ids.Count>1&&ids.Contains(who))targets=new List<long>{who};else break;
 }
 busy=false;}
 void OnDestroy(){if(texture!=null)UnityEngine.Object.Destroy(texture);}
}
}
