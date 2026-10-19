using System;
using System.Collections;
using System.Reflection;
using UnityEngine;
public sealed class SoulFormationDiagnostics : MonoBehaviour {
 object page;
 static object Field(object o,string n){return o.GetType().GetField(n,BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic).GetValue(o);}
 public static void Attach(object p){try{var c=(Component)p;var d=c.gameObject.AddComponent<SoulFormationDiagnostics>();d.page=p;d.StartCoroutine(d.Observe());}catch(Exception e){Debug.Log("FORMDIAG attach "+e);}}
 IEnumerator Observe(){for(int i=0;i<6;i++){yield return new WaitForSeconds(2);Dump(page,"tick"+i);}}
 public static void Dump(object p,string phase){try{
 var c=(Component)p;Debug.Log("FORMDIAG "+phase+" page="+Path(c.transform)+" active="+c.gameObject.activeInHierarchy);
 var list=(IEnumerable)Field(p,"models");int count=0;foreach(object x in list){var g=x as GameObject;if(g==null)continue;count++;Debug.Log("FORMDIAG model "+State(g.transform));var rr=g.GetComponentsInChildren<Renderer>(true);Debug.Log("FORMDIAG renderers="+rr.Length);foreach(var r in rr){Debug.Log("FORMDIAG renderer "+State(r.transform)+" enabled="+r.enabled+" visible="+r.isVisible+" bounds="+r.bounds+" order="+r.sortingOrder);foreach(var m in r.sharedMaterials){if(m!=null)Debug.Log("FORMDIAG material="+m.name+" shader="+(m.shader==null?"null":m.shader.name)+" queue="+m.renderQueue);}}}Debug.Log("FORMDIAG count="+count);
 foreach(var t in (Transform[])Field(p,"modelTransform"))Debug.Log("FORMDIAG slot "+State(t));
 foreach(Camera cam in Resources.FindObjectsOfTypeAll(typeof(Camera))){Debug.Log("FORMDIAG camera "+State(cam.transform)+" enabled="+cam.enabled+" depth="+cam.depth+" mask="+cam.cullingMask+" near="+cam.nearClipPlane+" far="+cam.farClipPlane+" ortho="+cam.orthographic+" size="+cam.orthographicSize);foreach(object x in list){var g=x as GameObject;if(g!=null)Debug.Log("FORMDIAG viewport="+cam.WorldToViewportPoint(g.transform.position)+" for="+g.name);}}
 }catch(Exception e){Debug.Log("FORMDIAG dump "+e);}}
 static string Path(Transform t){string p="";for(int i=0;t!=null&&i<12;i++,t=t.parent)p=t.name+"/"+p;return p;}
 static string State(Transform t){return Path(t)+" active="+t.gameObject.activeSelf+"/"+t.gameObject.activeInHierarchy+" layer="+t.gameObject.layer+" pos="+t.position+" local="+t.localPosition+" scale="+t.localScale+" lossy="+t.lossyScale;}
}
