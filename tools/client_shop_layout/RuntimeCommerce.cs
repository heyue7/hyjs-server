using System;
using System.Reflection;
using ProtoBuf.Message;
using UnityEngine;
using UI;

namespace HYJS.Feedback66
{
 public static class CommerceRuntime
 {
  static object Get(object row, string name) { return row.GetType().GetField(name, BindingFlags.Instance|BindingFlags.Public|BindingFlags.NonPublic).GetValue(row); }
  // Reuse the row's original label and button typography, without changing its purchase handler or enabled state.
  public static void Quota(object owner, object value)
  {
   Component row=owner as Component; ShopItem item=value as ShopItem;
   if(row==null || item==null || !(owner is ShopPopularityItem || owner is ShopPointItem)) return;
   Transform previous=row.transform.Find("DailyQuota66");
   if(item.condition!=null && item.condition.type=="rank") { if(previous!=null) previous.gameObject.SetActive(false); return; }
   UILabel style=Get(owner,"labName") as UILabel;
   GameObject button=Get(owner,"btnBuy") as GameObject;
   GameObject icon=Get(owner,"iconRoot") as GameObject;
   if(style==null || button==null || icon==null) return;
   // Original popularity/point cards have direct sibling widgets in row-local units.
   if(style.transform.parent!=row.transform || button.transform.parent!=row.transform || icon.transform.parent!=row.transform) return;
   UILabel label;
   if(previous==null)
   {
    GameObject clone=(GameObject)UnityEngine.Object.Instantiate(style.gameObject);
    clone.name="DailyQuota66"; clone.transform.parent=row.transform;
    clone.transform.localRotation=Quaternion.identity; clone.transform.localScale=Vector3.one;
    foreach(Collider collider in clone.GetComponents<Collider>()) UnityEngine.Object.Destroy(collider);
    label=clone.GetComponent<UILabel>();
   }
   else label=previous.GetComponent<UILabel>();
   if(label==null) return;
   Vector3 action=row.transform.InverseTransformPoint(button.transform.position);
   // Original card Condition label is Left-pivot at x=-146 in row coordinates.
   float left=-146f;
   label.leftAnchor.target=null; label.rightAnchor.target=null; label.topAnchor.target=null; label.bottomAnchor.target=null;
   label.pivot=UIWidget.Pivot.Left; label.alignment=NGUIText.Alignment.Left;
   label.fontSize=Math.Min(style.fontSize,24); label.maxLineCount=1;
   label.width=Math.Max(1,(int)(action.x-65f-left-10f)); label.height=Math.Max(30,label.fontSize+6);
   label.overflowMethod=UILabel.Overflow.ShrinkContent;
   label.text="今日剩余 "+Math.Max(0,item.dayLeftTimes)+"/"+Math.Max(0,item.dayTotalTimes);
   // Apply position last: UILabel geometry setters may move cloned widgets.
   label.transform.localPosition=new Vector3(left,action.y,style.transform.localPosition.z);
   label.gameObject.SetActive(item.dayTotalTimes>0);
  }
 }
}
