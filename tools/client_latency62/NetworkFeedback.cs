using System;
using System.Reflection;
using UnityEngine;
namespace HYJS.Latency62 {
public static class NetworkFeedback {
 public static void Init(object controller) {
  Component source=controller as Component;
  if(source==null)return;
  FeedbackView view=source.gameObject.GetComponent<FeedbackView>();
  if(view==null)view=source.gameObject.AddComponent<FeedbackView>();
  FieldInfo field=controller.GetType().GetField("m_kGameObject",BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic);
  GameObject visual=field==null?null:field.GetValue(controller) as GameObject;
  BringFront(source.gameObject);
  view.Begin(visual);
 }
 public static void BringFront(object value) {
  GameObject box=value as GameObject;
  if(box==null)return;
  FrontLayer front=box.GetComponent<FrontLayer>();
  if(front==null)front=box.AddComponent<FrontLayer>();
  front.Begin();
 }
 public static void Update(object controller) {
  Component source=controller as Component;
  if(source==null)return;
  FeedbackView view=source.gameObject.GetComponent<FeedbackView>();
  if(view!=null)view.Advance();
 }
}
public sealed class FeedbackView : MonoBehaviour {
 GameObject visual;
 UIWidget[] widgets;
 float[] originalAlpha;
 Renderer[] renderers;
 bool[] originalRenderer;
 GameObject compactObject;
 UITexture compact;
 float started;
 bool pending;
 public void Begin(GameObject next) {
  Restore();
  visual=next;
  if(visual==null)return;
  widgets=visual.GetComponentsInChildren<UIWidget>(true);
  originalAlpha=new float[widgets.Length];
  for(int i=0;i<widgets.Length;i++)originalAlpha[i]=widgets[i].alpha;
  renderers=visual.GetComponentsInChildren<Renderer>(true);
  originalRenderer=new bool[renderers.Length];
  for(int i=0;i<renderers.Length;i++)originalRenderer[i]=renderers[i].enabled;
  if(compactObject==null)CreateCompact();
  visual.SetActive(false);
  if(compactObject!=null)compactObject.SetActive(false);
  started=Time.realtimeSinceStartup;
  pending=true;
 }
 void CreateCompact() {
  UITexture source=null;
  for(int i=0;i<widgets.Length;i++) {
   UITexture t=widgets[i] as UITexture;
   if(t!=null && t.mainTexture!=null && t.mainTexture.name=="loading_xiaoquan") { source=t;break; }
  }
  if(source==null)return; // Retain original full feedback if an unexpected prefab has no ring.
  compactObject=new GameObject("HYJSCompactNetworkWait");
  compactObject.layer=source.gameObject.layer;
  compactObject.transform.parent=transform;
  compactObject.transform.localPosition=new Vector3(0f,0f,0f);
  compactObject.transform.localScale=Vector3.one;
  compact=compactObject.AddComponent<UITexture>();
  compact.mainTexture=source.mainTexture;
  compact.shader=source.shader;
  compact.color=Color.white;
  compact.depth=source.depth+1;
  compact.width=66;compact.height=66;
  compact.alpha=0f;
 }
 public void Advance() {
  if(!pending || visual==null)return;
  float age=Mathf.Max(0f,Time.realtimeSinceStartup-started);
  float small=Policy.Compact(age),full=Policy.Full(age);
  if(compactObject!=null) {
   compactObject.SetActive(small>0f);
   compact.alpha=small;
   compactObject.transform.localRotation=Quaternion.Euler(0f,0f,-age*150f);
  }
  if(full>0f) {
   if(!visual.activeSelf)visual.SetActive(true);
   for(int i=0;i<widgets.Length;i++)if(widgets[i]!=null)widgets[i].alpha=originalAlpha[i]*full;
   // NGUI alpha does not affect original ParticleSystemRenderer.
   for(int i=0;i<renderers.Length;i++)if(renderers[i]!=null)renderers[i].enabled=originalRenderer[i] && full>=1f;
  }
 }
 void Restore() {
  if(widgets!=null)for(int i=0;i<widgets.Length;i++)if(widgets[i]!=null)widgets[i].alpha=originalAlpha[i];
  if(renderers!=null)for(int i=0;i<renderers.Length;i++)if(renderers[i]!=null)renderers[i].enabled=originalRenderer[i];
 }
 void OnDisable() {
  pending=false;
  if(compactObject!=null)compactObject.SetActive(false);
  if(visual!=null)visual.SetActive(false);
  Restore();
 }
 void OnDestroy() { pending=false; }
}
}
