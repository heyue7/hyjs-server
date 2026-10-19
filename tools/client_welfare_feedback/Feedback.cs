using System;
using System.Collections.Generic;
using UnityEngine;
using UI;
namespace HYJS.WelfareFeedback
{
 public static class FeedbackBridge
 {
  // Move a newly claimed item into the claimed bucket without resetting scroll.
  public static void AchievementClaimed(AchievementItem item)
  {
   AchievementPage page=item.GetComponentInParent<AchievementPage>();
   if(page==null||page.grid==null)return;
   List<GameObject> ordered=new List<GameObject>();
   for(int bucket=0;bucket<3;bucket++)foreach(GameObject child in page.grid.childs)
   {
    AchievementItem row=child.GetComponent<AchievementItem>();
    if(row==null)continue;
    int group=row.finishBtn!=null&&row.finishBtn.activeSelf?0:row.goBtn!=null&&row.goBtn.activeSelf?1:2;
    if(group==bucket)ordered.Add(child);
   }
   if(ordered.Count==page.grid.childs.Count){page.grid.childs.Clear();page.grid.childs.AddRange(ordered);page.grid.Reposition();}
  }
 }
}
