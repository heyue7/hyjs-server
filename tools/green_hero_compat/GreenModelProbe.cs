using System;using System.IO;using System.Collections;using System.Reflection;using UnityEngine;
// Temporary isolated-view probe: no protocol send, profile, inventory or battle calls.
public static class AssetStateProbe {
 static bool running;
 public static void TryStart(MonoBehaviour owner){if(running||owner==null)return;running=true;owner.StartCoroutine(Run());}
 static string Q(string s){return "\""+(s??"").Replace("\\","\\\\").Replace("\"","\\\"")+"\"";}
 static void Log(string s){File.AppendAllText(Path.Combine(Application.persistentDataPath,"green-model-probe.jsonl"),s+"\n");}
 static void Layer(Transform t){t.gameObject.layer=31;foreach(Transform c in t)Layer(c);}
 static IEnumerator Run(){
  string[] models={"monster_068","monster_067","monster_070","lingnai01_body","sicary01_body","nio01_body"};
  string[] clips={"attack01","standfight","runfight","standweak","hit","dizzy","floata","falla","fallb","fallc","jumpb"};
  foreach(string model in models){GameObject clone=null,co=null,lo=null;AssetBundle bundle=null;bool own=false;RenderTexture rt=null;Texture2D image=null;
   try{
    string disk=Path.Combine(Application.persistentDataPath,"model/"+model);
    if(!File.Exists(disk))disk=Application.dataPath+"!assets/android/model/"+model;
    bundle=AssetBundle.LoadFromFile(disk);own=bundle!=null;GameObject prefab=null;
    if(bundle!=null){prefab=bundle.LoadAsset(model,typeof(GameObject)) as GameObject;if(prefab==null){UnityEngine.Object[] values=bundle.LoadAllAssets(typeof(GameObject));foreach(UnityEngine.Object value in values){GameObject go=value as GameObject;if(go!=null&&go.GetComponentInChildren<Animation>()!=null){prefab=go;break;}}}}
    if(prefab==null){Type t=typeof(PlaybackKit).Assembly.GetType("AssetLoad.AssetBundleManager",true);object manager=t.GetProperty("Instance",BindingFlags.Public|BindingFlags.Static).GetValue(null,null);prefab=t.GetMethod("GetAssetFromBundle",BindingFlags.Public|BindingFlags.NonPublic|BindingFlags.Instance).Invoke(manager,new object[]{"model/"+model,model,typeof(GameObject)}) as GameObject;}
    if(prefab==null)throw new Exception("original Unity prefab unavailable");
    if(prefab.GetComponentsInChildren<MonoBehaviour>(true).Length!=0)throw new Exception("prefab carries behavior; isolated instantiate rejected");
    clone=UnityEngine.Object.Instantiate(prefab) as GameObject;clone.name="HYJS-Green-Probe-"+model;clone.transform.position=new Vector3(0,-10000,0);Layer(clone.transform);clone.SetActive(true);
    Animation animation=clone.GetComponentInChildren<Animation>();if(animation==null)throw new Exception("Animation missing");
    foreach(string name in clips){AnimationState state=animation[name];if(state==null||state.length<=0)throw new Exception("clip missing: "+name);bool played=animation.Play(name);state.time=state.length*0.45f;animation.Sample();if(!played)throw new Exception("clip cannot play: "+name);Log("{\"kind\":\"clip\",\"model\":"+Q(model)+",\"name\":"+Q(name)+",\"played\":true,\"length\":"+state.length.ToString(System.Globalization.CultureInfo.InvariantCulture)+"}");}
    Renderer[] renderers=clone.GetComponentsInChildren<Renderer>(true);if(renderers.Length==0)throw new Exception("Renderer missing");
    co=new GameObject("HYJS-Green-Probe-Camera");Camera camera=co.AddComponent<Camera>();camera.cullingMask=1<<31;camera.clearFlags=CameraClearFlags.SolidColor;camera.backgroundColor=new Color(0.12f,0.15f,0.20f,1);camera.orthographic=true;camera.orthographicSize=1.5f;camera.nearClipPlane=0.01f;camera.farClipPlane=20;co.transform.position=new Vector3(0,-9999.1f,-4);co.transform.LookAt(new Vector3(0,-9999.1f,0));
    lo=new GameObject("HYJS-Green-Probe-Light");Light light=lo.AddComponent<Light>();light.type=LightType.Directional;light.cullingMask=1<<31;light.intensity=1.2f;lo.transform.rotation=Quaternion.Euler(45,-30,0);
    rt=RenderTexture.GetTemporary(256,384,24);camera.targetTexture=rt;camera.Render();RenderTexture previous=RenderTexture.active;try{RenderTexture.active=rt;image=new Texture2D(256,384,TextureFormat.RGB24,false);image.ReadPixels(new Rect(0,0,256,384),0,0);image.Apply();File.WriteAllBytes(Path.Combine(Application.persistentDataPath,"green-model-"+model+".png"),image.EncodeToPNG());}finally{RenderTexture.active=previous;}
    Log("{\"kind\":\"model\",\"model\":"+Q(model)+",\"instantiated\":true,\"rendererCount\":"+renderers.Length+",\"renderTextureCaptured\":true}");
   }catch(Exception e){Log("{\"kind\":\"error\",\"model\":"+Q(model)+",\"message\":"+Q(e.GetType().FullName+": "+e.Message)+"}");}
   finally{if(clone!=null)UnityEngine.Object.Destroy(clone);if(co!=null)UnityEngine.Object.Destroy(co);if(lo!=null)UnityEngine.Object.Destroy(lo);if(image!=null)UnityEngine.Object.Destroy(image);if(rt!=null)RenderTexture.ReleaseTemporary(rt);if(own&&bundle!=null)bundle.Unload(false);}
   yield return null;
  }
  Log("{\"kind\":\"complete\"}");running=false;
 }
}
