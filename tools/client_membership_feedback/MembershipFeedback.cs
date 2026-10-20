using UnityEngine;
using UI;
using ProtoBuf.Message;
namespace HYJS.Membership
{
 public static class MembershipFeedback
 {
  static void Caption(GameObject target, string text, UILabel theme, bool button)
  {
   if(target==null || theme==null)return;
   UISprite root=target.GetComponent<UISprite>();
   foreach(UISprite sprite in target.GetComponentsInChildren<UISprite>(true)) if(!button || sprite!=root) sprite.enabled=false;
   Transform child=target.transform.Find("MembershipCaption"); UILabel label;
   if(child==null){GameObject go=new GameObject("MembershipCaption");go.layer=target.layer;go.transform.parent=target.transform;go.transform.localPosition=Vector3.zero;go.transform.localScale=Vector3.one;label=go.AddComponent<UILabel>();}
   else label=child.GetComponent<UILabel>();
   if(theme.bitmapFont!=null)label.bitmapFont=theme.bitmapFont;else label.trueTypeFont=theme.trueTypeFont;
   label.text=text;label.fontSize=24;label.width=160;label.height=50;label.pivot=UIWidget.Pivot.Center;label.alignment=NGUIText.Alignment.Center;label.color=button?Color.white:new Color(.35f,.25f,.1f,1f);label.depth=root==null?theme.depth+10:root.depth+5;
  }
  public static void Show(VipBenefitDailyReward row, ActivityDailyBoonItem item, bool preview)
  {
   if(row==null||item==null||preview)return;
   int status=item.status;
   if(row.btnGet!=null){row.btnGet.SetActive(status==1);if(status==1)Caption(row.btnGet,"领取",row.labTitle,true);}
   if(row.getted!=null){row.getted.SetActive(status==2);if(status==2)Caption(row.getted,"已领取",row.labTitle,false);}
  }
 }
}
