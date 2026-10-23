using System;
using System.Globalization;
using UnityEngine;
using ProtoBuf.Message;
using NetWork.Packets;

namespace HYJS.PaymentFeedback
{
    public sealed class PaymentFeedback : MonoBehaviour
    {
        sealed class Attempt
        {
            public string Id, Product, Order;
            public long Player, Generation;
            public byte[] Session;
            public float Deadline;
            public double Amount;
            public bool Generating, Querying;
        }
        static PaymentFeedback instance;
        Attempt current;
        long generation;
        const string Unconfirmed = "支付结果尚未确认，可在蓝绿终端查看";
        static void Notice(string text) { BoxManager.CreatePopupTextBox(text, 0f); }
        static PaymentFeedback Host()
        {
            if (instance == null)
            {
                GameObject owner = new GameObject("HYJSPaymentFeedback");
                DontDestroyOnLoad(owner);
                instance = owner.AddComponent<PaymentFeedback>();
                // Retire the old singleton's UI-only work once on upgrade.
                PlatformSDKCallBack legacy = PlatformSDKCallBack.Instance;
                if (legacy != null)
                {
                    legacy.CancelInvoke();
                    foreach (string field in new string[] { "orderId", "payMentId", "tick" })
                    {
                        System.Reflection.FieldInfo info = typeof(PlatformSDKCallBack).GetField(field, System.Reflection.BindingFlags.Instance | System.Reflection.BindingFlags.Public | System.Reflection.BindingFlags.NonPublic);
                        if (info != null) info.SetValue(legacy, field == "tick" ? (object)(long)0 : (object)String.Empty);
                    }
                }
            }
            return instance;
        }
        bool SameSession(Attempt a)
        {
            if (StaticData.playerData == null || StaticData.playerData.playerId != a.Player) return false;
            byte[] now = StaticData.sessionKey;
            if (now == null || now.Length != a.Session.Length) return false;
            for (int i = 0; i < now.Length; i++) if (now[i] != a.Session[i]) return false;
            return true;
        }
        bool Active(Attempt a)
        {
            return a != null && System.Object.ReferenceEquals(current, a) && a.Generation == generation && SameSession(a);
        }
        bool Accept(Attempt a)
        {
            if (!Active(a)) return false;
            if (Time.realtimeSinceStartup >= a.Deadline) { Finish(a, Unconfirmed); return false; }
            return true;
        }
        void Finish(Attempt a, string text)
        {
            if (!Active(a)) return;
            current = null;
            if (text != null) Notice(text);
        }
        public static void Begin(UI.ChargeData owner, string name, string money, string product)
        {
            try { Host().StartAttempt(money, product); }
            catch (Exception) { Notice("无法发起充值，请稍后重试"); }
        }
        void StartAttempt(string money, string product)
        {
            if (current != null && !SameSession(current)) { current = null; generation++; }
            if (current != null)
            {
                if (Time.realtimeSinceStartup >= current.Deadline) Finish(current, Unconfirmed);
                else { Notice("正在确认当前充值，请稍候"); return; }
            }
            decimal amount;
            if (!Decimal.TryParse(money, NumberStyles.Number, CultureInfo.InvariantCulture, out amount) ||
                amount <= 0 || amount > Int64.MaxValue / 100m || Decimal.Truncate(amount * 100m) != amount * 100m ||
                String.IsNullOrEmpty(product) || StaticData.playerData == null || StaticData.playerData.playerId <= 0 ||
                StaticData.sessionKey == null || StaticData.sessionKey.Length != 16)
            { Notice("商品信息无效，请重新打开充值页"); return; }
            Attempt a = new Attempt();
            a.Id = Guid.NewGuid().ToString("D"); a.Product = product; a.Player = StaticData.playerData.playerId;
            a.Session = (byte[])StaticData.sessionKey.Clone(); a.Generation = ++generation;
            a.Amount = (double)amount; a.Deadline = Time.realtimeSinceStartup + 30f; current = a;
            try
            {
                using (AndroidJavaClass wallet = new AndroidJavaClass("com.hyjs.wallet.WalletActivity"))
                    wallet.CallStatic("checkRechargeBalance", a.Id, (long)(amount * 100m));
            }
            catch (Exception) { Finish(a, "读取超频点数失败，请稍后重试"); }
        }
        public void OnRechargeBalance(string value)
        {
            Attempt a = current;
            if (!Accept(a) || value == null) return;
            string[] parts = value.Split('|');
            if (parts.Length != 2 || parts[0] != a.Id || a.Generating) return;
            if (parts[1] == "insufficient") { Finish(a, "超频点数不足，无法购买该商品。"); return; }
            if (parts[1] != "ready") { Finish(a, "读取超频点数失败，请稍后重试"); return; }
            a.Generating = true;
            CSGenerateOrder request = new CSGenerateOrder();
            request.paymentId = a.Product; request.money = a.Amount; request.moneyType = "RMB";
            request.channelId = GameConfig.Instance.GetChannelId; request.shopId = "common";
            try { NetAPI.SendHttp(20042, request, delegate(Packet packet, bool success) { Generated(a, packet, success); }); }
            catch (Exception) { Finish(a, Unconfirmed); }
        }
        void Generated(Attempt a, Packet packet, bool success)
        {
            if (!Accept(a) || a.Querying) return;
            SCGenerateOrder reply = success && packet != null ? packet.kBody as SCGenerateOrder : null;
            if (reply == null || reply.transactionData == null || String.IsNullOrEmpty(reply.orderId) ||
                reply.transactionData.orderId != reply.orderId || reply.transactionData.paymentId != a.Product)
            { Finish(a, Unconfirmed); return; }
            a.Order = reply.orderId; a.Querying = true;
            CSCheckOrder request = new CSCheckOrder(); request.orderId = a.Order; request.paymentId = a.Product;
            try { NetAPI.SendHttp(20019, request, delegate(Packet result, bool ok) { Checked(a, result, ok); }); }
            catch (Exception) { Finish(a, Unconfirmed); }
        }
        void Checked(Attempt a, Packet packet, bool success)
        {
            if (!Accept(a)) return;
            SCCheckOrder reply = success && packet != null ? packet.kBody as SCCheckOrder : null;
            if (reply == null || reply.transactionData == null || reply.transactionData.orderId != a.Order ||
                reply.transactionData.paymentId != a.Product) { Finish(a, Unconfirmed); return; }
            if (reply.transactionData.payStatus == "PAY_STATUS_WAIT")
            { Finish(a, "订单尚未支付，可在蓝绿终端查看"); return; }
            if (reply.transactionData.payStatus != "PAY_STATUS_SUCCESS") { Finish(a, Unconfirmed); return; }
            Finish(a, "购买成功");
            UI.PageManager manager = UI.PageManager.Instance;
            UI.ChargePage page = manager == null ? null : manager.currentPage as UI.ChargePage;
            if (page != null)
            {
                page.InitTop();
                page.GetComponent<UI.ChargePage>().chargeData.GetComponent<UI.ChargeData>().InitPayMent();
            }
        }
        void Update()
        {
            Attempt a = current;
            if (a == null) return;
            if (!SameSession(a)) { current = null; generation++; return; }
            if (Time.realtimeSinceStartup >= a.Deadline) { Finish(a, Unconfirmed); }
        }
    }
}
