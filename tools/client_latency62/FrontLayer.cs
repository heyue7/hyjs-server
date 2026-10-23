using System;
using UnityEngine;
namespace HYJS.Latency62 {
// Snapshot per display; no global panel scan from Update or any coroutine.
public sealed class FrontLayer : MonoBehaviour {
 UIPanel[] panels;
 int[] depths,orders;
 public void Begin() {
  Restore();
  panels=gameObject.GetComponentsInChildren<UIPanel>(true);
  if(panels.Length==0){panels=null;return;}
  depths=new int[panels.Length];orders=new int[panels.Length];
  int minDepth=int.MaxValue,minOrder=int.MaxValue;
  for(int i=0;i<panels.Length;i++) {
   depths[i]=panels[i].depth;orders[i]=panels[i].sortingOrder;
   minDepth=Math.Min(minDepth,depths[i]);minOrder=Math.Min(minOrder,orders[i]);
  }
  int maxDepth=int.MinValue,maxOrder=int.MinValue;
  foreach(UIPanel other in UIPanel.list) {
   if(other==null || !other.enabled || !other.gameObject.activeInHierarchy)continue;
   bool own=false;for(int i=0;i<panels.Length;i++)if(other==panels[i]){own=true;break;}
   if(own)continue;
   maxDepth=Math.Max(maxDepth,other.depth);maxOrder=Math.Max(maxOrder,other.sortingOrder);
  }
  int depthDelta=maxDepth>=minDepth?maxDepth-minDepth+1:0;
  int orderDelta=maxOrder>=minOrder?maxOrder-minOrder+1:0;
  for(int i=0;i<panels.Length;i++) {
   panels[i].depth=depths[i]+depthDelta;
   panels[i].sortingOrder=orders[i]+orderDelta;
  }
 }
 void Restore() {
  if(panels==null)return;
  for(int i=0;i<panels.Length;i++)if(panels[i]!=null){panels[i].depth=depths[i];panels[i].sortingOrder=orders[i];}
  panels=null;depths=null;orders=null;
 }
 void OnDisable(){Restore();}
 void OnDestroy(){Restore();}
}
}
