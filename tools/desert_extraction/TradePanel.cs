using System;
using System.Globalization;
using System.Reflection;
using UI;
using UnityEngine;

namespace HYJS.DesertV4
{
    // Native SAO message panel, CommonIcon item cells and activity quantity
    // popup; each visual amount is taken from the server's quote.
    public sealed class TradePanel : MonoBehaviour
    {
        static TradePanel instance;
        static readonly string[] Colors={"白","绿","蓝","紫","橙","红"};
        GameObject root,amountRoot;
        UILabel title,ownText,peerText,valuesText,protectionText;
        CommonIcon[] own=new CommonIcon[6],peer=new CommonIcon[6];
        TradeQuote quote;
        long playerId,amount,maximum,amountVersion;
        int amountIndex;
        bool busy;
        int holdDelta;
        float holdNext;
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text,0f); }
        static void Unlocalize(UILabel label)
        {
            if (label==null) return;
            foreach (MonoBehaviour component in label.GetComponents<MonoBehaviour>())
                if (component!=null && component.GetType().Name=="TextKit") DestroyImmediate(component);
        }
        static void ClearHandlers(GameObject go)
        {
            foreach (UIButtonMessage b in go.GetComponentsInChildren<UIButtonMessage>(true)) DestroyImmediate(b);
            foreach (UIEventListener e in go.GetComponentsInChildren<UIEventListener>(true)) DestroyImmediate(e);
            foreach (UIButton button in go.GetComponentsInChildren<UIButton>(true)) button.onClick.Clear();
        }
        static GameObject Find(GameObject root,string path)
        { Transform t=root.transform.Find(path); if (t==null) throw new InvalidOperationException("Missing original UI component: "+path); return t.gameObject; }
        static void Click(GameObject go,Action action)
        { UIEventListener.Get(go).onClick=delegate(GameObject ignored) { action(); }; }
        // NGUI renders by sortingOrder and uses panel depth for hit testing.
        // Move every panel together so the native mask and children keep their order.
        static void RaiseAbove(GameObject front,GameObject back)
        {
            if (front==null || back==null) throw new InvalidOperationException("Native modal unavailable");
            UIPanel[] upper=front.GetComponentsInChildren<UIPanel>(true), lower=back.GetComponentsInChildren<UIPanel>(true);
            if (upper.Length==0 || lower.Length==0) throw new InvalidOperationException("Native panel ordering unavailable");
            int minDepth=Int32.MaxValue,minOrder=Int32.MaxValue,maxDepth=Int32.MinValue,maxOrder=Int32.MinValue;
            foreach (UIPanel panel in upper) { minDepth=Math.Min(minDepth,panel.depth); minOrder=Math.Min(minOrder,panel.sortingOrder); }
            foreach (UIPanel panel in lower) { maxDepth=Math.Max(maxDepth,panel.depth); maxOrder=Math.Max(maxOrder,panel.sortingOrder); }
            int depthShift=checked((int)Math.Max(0L,(long)maxDepth-minDepth+1));
            int orderShift=checked((int)Math.Max(0L,(long)maxOrder-minOrder+1));
            foreach (UIPanel panel in upper)
            { panel.depth=checked(panel.depth+depthShift); panel.sortingOrder=checked(panel.sortingOrder+orderShift); }
        }
        void RaiseConfirmation()
        {
            GameObject modal=BoxManager.m_kBox;
            try { RaiseAbove(modal,root); }
            catch (Exception e)
            {
                if (modal!=null) modal.SetActive(false);
                BoxManager.CleanAllBox();
                Debug.LogWarning("Trade confirmation: "+e.GetType().FullName+": "+e.Message);
                Notice("确认窗口暂不可用，请稍后重试");
            }
        }
        static UILabel CloneLabel(UILabel template,Transform parent,string name,Vector3 position,int size,int width)
        {
            GameObject item=(GameObject)Instantiate(template.gameObject); item.name=name; item.transform.parent=parent;
            item.transform.localScale=Vector3.one; item.transform.localRotation=Quaternion.identity; item.transform.localPosition=position;
            foreach (Transform child in item.transform) Destroy(child.gameObject);
            UILabel label=item.GetComponent<UILabel>(); Unlocalize(label); label.fontSize=size; label.width=width; label.gameObject.SetActive(true); return label;
        }
        static void SetButtonText(GameObject button,UILabel template,string artPath,string text)
        {
            // These original buttons draw their captions as sprites, not UILabels.
            // Reuse the native label font and the caption sprite's position/depth.
            GameObject art=Find(button,artPath); UIWidget caption=art.GetComponent<UIWidget>();
            if (template==null || caption==null) throw new InvalidOperationException("Native button caption unavailable");
            UILabel label=CloneLabel(template,button.transform,"DesertTradeButtonText",art.transform.localPosition,20,160);
            label.depth=caption.depth+1; label.alignment=NGUIText.Alignment.Center;
            label.overflowMethod=UILabel.Overflow.ShrinkContent; label.color=Color.white; label.text=text;
            art.SetActive(false);
        }
        internal static void RegisterIcons()
        {
            Type t=Type.GetType("HYJS.GGO.GGOShop, SoulFlowerGGOShop",false);
            MethodInfo method=t==null ? null:t.GetMethod("RegisterIcons",BindingFlags.Static|BindingFlags.NonPublic);
            if (method==null) throw new InvalidOperationException("Original chip icon registration unavailable");
            method.Invoke(null,null);
        }
        static TradePanel Create()
        {
            RegisterIcons();
            GameObject prefab=Resources.Load("UI/MessageBox/MessageBoxWithConfirmAndCancelButton") as GameObject;
            if (prefab==null || BoxManager.Anthor==null) throw new InvalidOperationException("Native message panel unavailable");
            GameObject root=(GameObject)Instantiate(prefab); root.name="DesertTradePanel";
            try
            {
            root.transform.parent=BoxManager.Anthor;
            root.transform.localPosition=Vector3.zero; root.transform.localScale=Vector3.one; root.transform.localRotation=Quaternion.identity;
            MessageBoxWidth2BtnController native=root.GetComponent<MessageBoxWidth2BtnController>();
            if (native==null || native.m_kLabel==null || native.m_kOKBtn==null || native.m_kCancelBtn==null || native.titleLable==null) { Destroy(root); throw new InvalidOperationException("Native message panel fields unavailable"); }
            native.enabled=false; ClearHandlers(root);
            TradePanel panel=root.AddComponent<TradePanel>(); panel.root=root;
            Transform main=Find(root,"Window/Main").transform;
            UIWidget bg=Find(root,"Window/BG").GetComponent<UIWidget>(); if (bg==null) { Destroy(root); throw new InvalidOperationException("Native panel background unavailable"); }
            bg.width=600; bg.height=414;
            panel.title=native.titleLable; Unlocalize(panel.title); panel.title.gameObject.SetActive(true); panel.title.text="芯片交易";
            panel.title.transform.localPosition=new Vector3(0,182,0);
            UILabel original=native.m_kLabel;
            panel.ownText=CloneLabel(original,main,"TradeSelf",new Vector3(0,151,0),20,510);
            panel.peerText=CloneLabel(original,main,"TradePeer",new Vector3(0,22,0),20,510);
            panel.valuesText=CloneLabel(original,main,"TradeValues",new Vector3(0,-92,0),18,540);
            panel.protectionText=CloneLabel(original,main,"TradeProtected",new Vector3(0,-121,0),17,540);
            original.gameObject.SetActive(false);
            native.m_kOKBtn.transform.localPosition=new Vector3(-105,-171,0);
            native.m_kCancelBtn.transform.localPosition=new Vector3(105,-171,0);
            SetButtonText(native.m_kOKBtn,original,"Label","确认交易"); SetButtonText(native.m_kCancelBtn,original,"Label","取消交易");
            Click(native.m_kOKBtn,delegate { panel.Confirm(); }); Click(native.m_kCancelBtn,delegate { panel.Cancel(); });
            for (int i=0;i<6;i++)
            {
                int color=i;
                CommonIcon a=CommonMethod.CreateCommonIcon(main),b=CommonMethod.CreateCommonIcon(main);
                if (a==null || b==null) { Destroy(root); throw new InvalidOperationException("Native item cell unavailable"); }
                a.transform.localScale=b.transform.localScale=Vector3.one*.47f;
                a.transform.localPosition=new Vector3(-195+i*78,103,0); b.transform.localPosition=new Vector3(-195+i*78,-27,0);
                a.SetData("item_970"+(i+1),false,-1,null); b.SetData("item_970"+(i+1),false,-1,null);
                ClearHandlers(a.gameObject); ClearHandlers(b.gameObject);
                NGUITools.AddWidgetCollider(a.gameObject);
                Click(a.gameObject,delegate { panel.OpenAmount(color); });
                panel.own[i]=a; panel.peer[i]=b;
            }
            root.SetActive(true); return panel;
            }
            catch (Exception) { Destroy(root); throw; }
        }
        public static void Show(TradeQuote q,long owner,bool waiting)
        {
            if (q==null || q.status!="open" || (owner!=q.initiator && owner!=q.receiver)) return;
            try
            {
                if (instance==null) instance=Create();
                instance.Render(q,owner,waiting);
            }
            catch (Exception e) { Debug.LogWarning("Trade panel: "+e.GetType().FullName+": "+e.Message); Notice("交易界面暂不可用，请稍后重试"); }
        }
        public static void CloseLocal()
        {
            if (instance==null) return; TradePanel old=instance; instance=null;
            old.CloseAmount(); if (old.root!=null) { old.root.SetActive(false); Destroy(old.root); }
        }
        void OnDestroy() { if (instance==this) instance=null; CloseAmount(); }
        void Render(TradeQuote q,long owner,bool waiting)
        {
            if (amountRoot!=null && (quote==null || q.id!=quote.id || q.version!=amountVersion)) CloseAmount();
            quote=q; playerId=owner; busy=waiting;
            int me=owner==q.initiator ? 0:1, other=1-me;
            ownText.text="我的付出"+(q.confirmed[me] ? "（已确认）" : "（点击芯片修改）");
            peerText.text="对方付出"+(q.confirmed[other] ? "（已确认）" : "（待确认）");
            valuesText.text="预计信用：我的 "+(String.IsNullOrEmpty(q.values[me]) ? "--":q.values[me])+"   对方 "+(String.IsNullOrEmpty(q.values[other]) ? "--":q.values[other]);
            string protection="成交后保护：";
            if (q.preview) for (int i=0;i<6;i++) protection+=Colors[i]+q.afterProtected[i].ToString(CultureInfo.InvariantCulture)+(i==5 ? "":"  ");
            else protection+="待双方填写有效数量";
            protectionText.text=protection;
            for (int i=0;i<6;i++) { own[i].UpdateCountNum(q.offers[me][i].ToString(CultureInfo.InvariantCulture)); peer[i].UpdateCountNum(q.offers[other][i].ToString(CultureInfo.InvariantCulture)); }
        }
        void Confirm()
        {
            if (amountRoot!=null) return;
            if (quote==null || busy || !quote.preview) { Notice("请等待有效的交易报价"); return; }
            long version=quote.version; string id=quote.id;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("确认当前双方数量和保护结果？",delegate(bool yes)
            { if (yes && quote!=null && quote.id==id && quote.version==version) TradeController.Action("confirm",null); else if (yes) Notice("报价已变化，请重新确认"); });
            RaiseConfirmation();
        }
        void Cancel()
        {
            if (busy || amountRoot!=null) return;
            BoxManager.CreateMessageBoxWithCofirmAndCancelButton("取消当前芯片交易？",delegate(bool yes)
            { if (yes) TradeController.Action("cancel",null); });
            RaiseConfirmation();
        }
        void OpenAmount(int index)
        {
            if (quote==null || busy || amountRoot!=null) return;
            try
            {
                GameObject prefab=Resources.Load("Pages/ActivityItemBatchBuyPage") as GameObject;
                if (prefab==null) throw new InvalidOperationException("Native quantity popup unavailable");
                amountRoot=(GameObject)Instantiate(prefab); amountRoot.name="DesertTradeAmount";
                amountRoot.transform.parent=BoxManager.Anthor; amountRoot.transform.localScale=Vector3.one;
                amountRoot.transform.localRotation=Quaternion.identity; amountRoot.transform.localPosition=new Vector3(0,0,-1200);
                ActivityItemBatchBuyPage native=amountRoot.GetComponent<ActivityItemBatchBuyPage>();
                if (native==null || native.labName==null || native.labHaveNum==null || native.labCurCount==null || native.iconRoot==null) throw new InvalidOperationException("Native quantity fields unavailable");
                native.enabled=false; ClearHandlers(amountRoot);
                int me=playerId==quote.initiator ? 0:1;
                amountIndex=index; amount=quote.offers[me][index]; maximum=quote.balance[index]; amountVersion=quote.version;
                if (amount>maximum) throw new InvalidOperationException("Offer exceeds balance");
                Unlocalize(native.labName); Unlocalize(native.labHaveNum); Unlocalize(native.labCurCount);
                native.labName.text=Colors[index]+"色芯片"; native.labHaveNum.text=maximum.ToString(CultureInfo.InvariantCulture);
                native.labCurCount.text=amount.ToString(CultureInfo.InvariantCulture);
                foreach (string path in new[]{"bg/Info/wordLeftTimes","bg/Info/LeftTimes","bg/Info/wordTotalPrice","bg/Info/Pirce","bg/Info/logo"}) Find(amountRoot,path).SetActive(false);
                UILabel heading=Find(amountRoot,"bg/Info/title").GetComponent<UILabel>(); if (heading!=null) { Unlocalize(heading); heading.text="选择数量"; }
                if (native.labDesc!=null) { Unlocalize(native.labDesc); native.labDesc.text="选择本次付出数量"; }
                NGUITools.DestroyChildren(native.iconRoot.transform); CommonIcon icon=CommonMethod.CreateCommonIcon(native.iconRoot.transform);
                if (icon==null) throw new InvalidOperationException("Native item icon unavailable"); icon.SetData("item_970"+(index+1),false,-1,null);
                BindStep(Find(amountRoot,"bg/AmontControl/Minu10"),-10); BindStep(Find(amountRoot,"bg/AmontControl/Minu1"),-1);
                BindStep(Find(amountRoot,"bg/AmontControl/Add1"),1); BindStep(Find(amountRoot,"bg/AmontControl/Add10"),10);
                GameObject confirm=Find(amountRoot,"bg/AmontControl/okButton"); SetButtonText(confirm,native.labCurCount,"leftAction","确定"); Click(confirm,SubmitAmount);
                Click(Find(amountRoot,"bg/Info/close"),CloseAmount);
                foreach (UIWidget widget in amountRoot.GetComponentsInChildren<UIWidget>(true)) widget.depth+=100;
                amountRoot.SetActive(true); RaiseAbove(amountRoot,root);
            }
            catch (Exception e) { Debug.LogWarning("Trade quantity: "+e.GetType().FullName+": "+e.Message); CloseAmount(); Notice("数量窗口暂不可用"); }
        }
        void BindStep(GameObject button,int delta)
        {
            UIEventListener.Get(button).onPress=delegate(GameObject go,bool pressed)
            { if (pressed) { holdDelta=delta; Step(delta); holdNext=Time.unscaledTime+.5f; } else holdDelta=0; };
        }
        void Update()
        {
            if (amountRoot!=null && holdDelta!=0 && Time.unscaledTime>=holdNext) { Step(holdDelta); holdNext=Time.unscaledTime+.08f; }
        }
        void Step(int delta)
        {
            if (amountRoot==null) return;
            if (delta>0) amount=maximum-amount<delta ? maximum:amount+delta;
            else amount=amount<-delta ? 0:amount+delta;
            ActivityItemBatchBuyPage native=amountRoot.GetComponent<ActivityItemBatchBuyPage>();
            if (native!=null && native.labCurCount!=null) native.labCurCount.text=amount.ToString(CultureInfo.InvariantCulture);
        }
        void SubmitAmount()
        {
            if (quote==null || quote.version!=amountVersion || busy) { CloseAmount(); Notice("报价已变化，请重新选择"); return; }
            int me=playerId==quote.initiator ? 0:1; long[] values=(long[])quote.offers[me].Clone(); values[amountIndex]=amount;
            CloseAmount(); TradeController.Action("offer",values);
        }
        void CloseAmount()
        {
            holdDelta=0; GameObject closing=amountRoot; amountRoot=null;
            if (closing!=null) { closing.SetActive(false); Destroy(closing); }
        }
    }
}
