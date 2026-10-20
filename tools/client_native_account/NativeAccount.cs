using System;
using System.Collections;
using System.Collections.Generic;
using UnityEngine;

namespace HYJS.NativeAccount
{
    // Keep the shipped NGUI panel, inputs, artwork and tweens in their original hierarchy.
    public static class AccountUI
    {
        public static void Show(UserInput input)
        {
            AccountControl control = input.GetComponent<AccountControl>();
            if (control == null) control = input.gameObject.AddComponent<AccountControl>();
            control.Open(input);
        }
        public static void Register(UserInput input)
        {
            ShowIfNeeded(input).Toggle();
        }
        static AccountControl ShowIfNeeded(UserInput input)
        {
            AccountControl value = input.GetComponent<AccountControl>();
            if (value == null) { Show(input); value = input.GetComponent<AccountControl>(); }
            return value;
        }
        public static bool HandleLogin(UserInput input)
        {
            AccountControl value = input.GetComponent<AccountControl>();
            return value != null && value.Submit();
        }
    }

    public sealed class AccountControl : MonoBehaviour
    {
        UserInput owner;
        UIButton login, register, remember;
        UILabel loginLabel, registerLabel, rememberLabel;
        UIInput confirmation;
        Vector3 namePosition, passwordPosition;
        bool registering, busy, initialized;
        int request;
        static Texture2D buttonTexture;
        static AndroidJavaClass Cache() { return new AndroidJavaClass("com.hyjs.wallet.LoginCache"); }
        static AndroidJavaClass Registration() { return new AndroidJavaClass("com.hyjs.wallet.GameRegistration"); }
        UIButton Find(string method)
        {
            foreach (UIButton button in owner.GetComponentsInChildren<UIButton>(true))
                foreach (EventDelegate action in button.onClick)
                    if (action.target == owner && action.methodName == method) return button;
            return null;
        }
        UILabel Label(UIButton button)
        {
            if (button == null) return null;
            // Reuse the shipped gold game button pixels inside the original account panel.
            UISprite background = button.GetComponent<UISprite>();
            foreach (UISprite sprite in button.GetComponentsInChildren<UISprite>(true)) if (sprite != background) sprite.enabled = false;
            // NGUI uses the root widget for hit testing and child alpha. Keep it active beneath the game skin.
            if (background != null) { background.enabled = true; background.color = Color.black; background.width = 156; background.height = 52; }
            if (buttonTexture == null)
            {
                using (System.IO.Stream stream = typeof(AccountControl).Assembly.GetManifestResourceStream("HYJS.AccountButton.png"))
                {
                    byte[] bytes = new byte[stream.Length];
                    int offset = 0, count;
                    while (offset < bytes.Length && (count = stream.Read(bytes, offset, bytes.Length - offset)) > 0) offset += count;
                    buttonTexture = new Texture2D(2, 2); buttonTexture.LoadImage(bytes);
                }
            }
            Transform skinChild = button.transform.Find("GameButtonSkin");
            if (skinChild == null)
            {
                GameObject go = new GameObject("GameButtonSkin"); go.layer = button.gameObject.layer;
                go.transform.parent = button.transform; go.transform.localPosition = Vector3.zero; go.transform.localScale = Vector3.one;
                skinChild = go.transform;
            }
            UITexture skin = skinChild.GetComponent<UITexture>();
            if (skin == null) skin = skinChild.gameObject.AddComponent<UITexture>();
            skin.mainTexture = buttonTexture; skin.width = 156; skin.height = 52;
            skin.depth = background == null ? 10 : background.depth + 1;
            button.tweenTarget = skin.gameObject;
            button.defaultColor = Color.white;
            Transform existing = button.transform.Find("NativeButtonCaption");
            if (existing != null) return existing.GetComponent<UILabel>();
            GameObject text = new GameObject("NativeButtonCaption"); text.layer = button.gameObject.layer;
            text.transform.parent = button.transform; text.transform.localPosition = Vector3.zero; text.transform.localScale = Vector3.one;
            UILabel label = text.AddComponent<UILabel>();
            UILabel theme = owner.nameInput.GetComponent<UIInput>().label;
            if (theme.bitmapFont != null) label.bitmapFont = theme.bitmapFont; else label.trueTypeFont = theme.trueTypeFont;
            label.material = theme.material;
            label.fontSize = 20; label.color = Color.white; label.width = 140; label.height = 40;
            label.effectStyle = UILabel.Effect.Outline; label.effectColor = Color.black;
            label.alignment = NGUIText.Alignment.Center; label.pivot = UIWidget.Pivot.Center;
            label.depth = skin.depth + 5;
            return label;
        }
        public void Open(UserInput input)
        {
            owner = input;
            if (!initialized)
            {
                initialized = true;
                namePosition = input.nameInput.transform.localPosition;
                passwordPosition = input.passWordInput.transform.localPosition;
                login = Find("OnLoginButton"); register = Find("OnRegistButtonClick");
                loginLabel = Label(login); registerLabel = Label(register);
                input.nameInput.GetComponent<UIInput>().defaultText = "账号：请输入账号";
                input.passWordInput.GetComponent<UIInput>().defaultText = "密码：请输入密码";
                // Reuse the original blue button for the cache preference; obsolete empty callbacks disappear.
                UIButton unused = Find("OnForgetPassWord");
                if (unused != null) unused.gameObject.SetActive(false);
                unused = Find("OnTouristsLogin"); if (unused != null) unused.gameObject.SetActive(false);
                if (register != null)
                {
                    GameObject copy = (GameObject)UnityEngine.Object.Instantiate(register.gameObject);
                    copy.name = "RememberPassword"; copy.transform.parent = register.transform.parent;
                    copy.transform.localScale = register.transform.localScale;
                    // Use the free original link row inside the panel, above its two action buttons.
                    copy.transform.localPosition = register.transform.localPosition + new Vector3(0f, 60f, 0f);
                    remember = copy.GetComponent<UIButton>(); remember.onClick.Clear();
                    EventDelegate.Add(remember.onClick, ChangeRemember);
                    rememberLabel = Label(remember);
                }
                GameObject confirm = (GameObject)UnityEngine.Object.Instantiate(input.passWordInput);
                confirm.name = "ConfirmPassword"; confirm.transform.parent = input.passWordInput.transform.parent;
                confirm.transform.localScale = input.passWordInput.transform.localScale;
                confirm.transform.localPosition = passwordPosition + Vector3.down * 52f;
                confirmation = confirm.GetComponent<UIInput>(); confirmation.value = "";
                confirmation.onSubmit.Clear(); confirmation.onChange.Clear();
                confirmation.defaultText = "确认密码：请再次输入";
                confirmation.UpdateLabel();
                confirm.SetActive(false);
            }
            registering = false; busy = false; Apply();
            StartCoroutine(RefreshPlaceholders());
        }
        IEnumerator RefreshPlaceholders()
        {
            // UIInput.Start captures its prefab text once; apply the account labels after that capture.
            yield return null;
            UIInput name = owner.nameInput.GetComponent<UIInput>();
            UIInput password = owner.passWordInput.GetComponent<UIInput>();
            name.defaultText = "账号：请输入账号";
            password.defaultText = "密码：请输入密码";
            confirmation.defaultText = "确认密码：请再次输入";
            name.UpdateLabel(); password.UpdateLabel(); confirmation.UpdateLabel();
        }
        void ChangeRemember()
        {
            using (AndroidJavaClass cache = Cache()) cache.CallStatic("setRemember", !cache.CallStatic<bool>("rememberEnabled"));
            Apply();
        }
        public void Toggle()
        {
            if (busy) return;
            registering = !registering; confirmation.value = ""; owner.ErrInfoFalse(); Apply();
        }
        void Apply()
        {
            // Fit the extra confirmation row in the same original panel; the title scene stays intact.
            owner.nameInput.transform.localPosition = namePosition;
            owner.passWordInput.transform.localPosition = passwordPosition;
            if (confirmation != null) { confirmation.transform.localPosition = passwordPosition + Vector3.down * 52f; confirmation.gameObject.SetActive(registering); }
            if (loginLabel != null) loginLabel.text = busy ? "注册中…" : registering ? "注册" : "登录";
            if (registerLabel != null) registerLabel.text = registering ? "返回登录" : "注册账号";
            if (remember != null)
            {
                remember.gameObject.SetActive(!registering);
                using (AndroidJavaClass cache = Cache()) if (rememberLabel != null) rememberLabel.text = cache.CallStatic<bool>("rememberEnabled") ? "记住密码：开" : "记住密码：关";
            }
        }
        void Error(string message) { BoxManager.CreatePopupTextBox(message, 0f); }
        public bool Submit()
        {
            if (!registering) return false;
            if (busy) return true;
            string name = owner.nameInput.GetComponent<UIInput>().value;
            string password = owner.passWordInput.GetComponent<UIInput>().value;
            string confirm = confirmation.value;
            if (!System.Text.RegularExpressions.Regex.IsMatch(name, "^[A-Za-z0-9_-]{3,32}$")) { Error("账号需为3至32位字母、数字、下划线或短横线"); return true; }
            if (password.Length < 6 || System.Text.Encoding.UTF8.GetByteCount(password) > 72) { Error("密码至少6个字符，最多72字节"); return true; }
            if (password != confirm) { Error("两次输入的密码不一样"); return true; }
            busy = true; Apply();
            using (AndroidJavaClass bridge = Registration()) request = bridge.CallStatic<int>("submitNative", ServerConfig.GetServerUrl(), name, password, confirm);
            StartCoroutine(WaitForRegistration(name, password, request)); return true;
        }
        IEnumerator WaitForRegistration(string name, string password, int identity)
        {
            float deadline = Time.realtimeSinceStartup + 25f;
            while (busy && identity == request && gameObject.activeInHierarchy)
            {
                int state; string message = "";
                using (AndroidJavaClass bridge = Registration()) { state = bridge.CallStatic<int>("nativeStatus", identity); if (state == 2) message = bridge.CallStatic<string>("nativeError", identity); }
                if (state != 0 || Time.realtimeSinceStartup >= deadline)
                {
                    busy = false;
                    if (state == 1)
                    {
                        registering = false; confirmation.value = "";
                        owner.nameInput.GetComponent<UIInput>().value = name;
                        owner.passWordInput.GetComponent<UIInput>().value = password;
                        Error("注册成功，请登录");
                    }
                    else Error(message == "" ? "注册未完成，请重试" : message);
                    Apply(); yield break;
                }
                yield return null;
            }
        }
        void OnDisable()
        {
            busy = false; ++request; StopAllCoroutines();
            using (AndroidJavaClass bridge = Registration()) bridge.CallStatic("cancelNative");
            if (confirmation != null) confirmation.value = "";
        }
    }
}
