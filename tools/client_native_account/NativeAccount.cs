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
        public static bool ResumeField()
        {
            string scene = Login.LoginManager.willLoginScene;
            if (scene != "field_001" && scene != "field_002") return false;
            // LoginToField accepts the original resource-dispatch prefix and strips it itself.
            // The authoritative protocol scene and saved position retain field_001/002.
            UI.UIFieldRunMap.fieldId = String.Empty;
            ManagerController.Instance.LoginToField("town_side_" + scene);
            return true;
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
        static UIAtlas buttonAtlas;
        UITexture rememberFace;
        GameObject rememberTick;
        static readonly Color Ink = new Color(0.19f, 0.28f, 0.33f, 1f);
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
            // Keep the original event/collider; skin only the small action control.
            UISprite background = button.GetComponent<UISprite>();
            foreach (UISprite sprite in button.GetComponentsInChildren<UISprite>(true)) if (sprite != background) sprite.enabled = false;
            // NGUI uses the root widget for hit testing and child alpha. Keep it active beneath the game skin.
            bool primary = button == login;
            if (background == null) return null;
            background.enabled = true;
            if (buttonTexture == null)
            {
                using (System.IO.Stream stream = typeof(AccountControl).Assembly.GetManifestResourceStream("HYJS.AccountButtons.png"))
                {
                    byte[] bytes = new byte[stream.Length];
                    int offset = 0, count;
                    while (offset < bytes.Length && (count = stream.Read(bytes, offset, bytes.Length - offset)) > 0) offset += count;
                    buttonTexture = new Texture2D(2, 2); buttonTexture.LoadImage(bytes);
                }
            }
            if (buttonAtlas == null)
            {
                GameObject resource = new GameObject("SAOAccountButtonAtlas");
                UnityEngine.Object.DontDestroyOnLoad(resource);
                buttonAtlas = resource.AddComponent<UIAtlas>();
                Material material = new Material(background.atlas.spriteMaterial); material.mainTexture = buttonTexture;
                buttonAtlas.spriteMaterial = material;
                List<UISpriteData> sprites = new List<UISpriteData>();
                string[] names = { "Primary", "Secondary", "PrimaryPressed", "SecondaryPressed" };
                for (int i = 0; i < 4; i++)
                {
                    UISpriteData data = new UISpriteData(); data.name = names[i];
                    data.x = (i % 2 == 0) ? 110 : 792; data.y = i < 2 ? 280 : 568; data.width = 636; data.height = 170;
                    sprites.Add(data);
                }
                buttonAtlas.spriteList = sprites;
            }
            string normal = primary ? "Primary" : "Secondary";
            background.atlas = buttonAtlas; background.type = UIBasicSprite.Type.Simple;
            background.spriteName = normal; background.width = 156; background.height = 44; background.color = Color.white;
            button.tweenTarget = button.gameObject;
            button.normalSprite = normal; button.hoverSprite = normal; button.pressedSprite = normal + "Pressed"; button.disabledSprite = normal;
            button.defaultColor = Color.white;
            button.hover = Color.white; button.pressed = Color.white;
            Transform existing = button.transform.Find("NativeButtonCaption");
            if (existing != null) return existing.GetComponent<UILabel>();
            GameObject text = new GameObject("NativeButtonCaption"); text.layer = button.gameObject.layer;
            text.transform.parent = button.transform; text.transform.localPosition = Vector3.zero; text.transform.localScale = Vector3.one;
            UILabel label = text.AddComponent<UILabel>();
            UILabel theme = owner.nameInput.GetComponent<UIInput>().label;
            if (theme.bitmapFont != null) label.bitmapFont = theme.bitmapFont; else label.trueTypeFont = theme.trueTypeFont;
            label.material = theme.material;
            label.fontSize = 20; label.color = primary ? Color.white : Ink; label.width = 140; label.height = 40;
            label.effectStyle = UILabel.Effect.None;
            label.alignment = NGUIText.Alignment.Center; label.pivot = UIWidget.Pivot.Center;
            label.depth = background.depth + 5;
            return label;
        }
        UITexture Solid(Transform parent, string name, int width, int height, Vector3 position, int depth, Color color)
        {
            GameObject go = new GameObject(name); go.layer = parent.gameObject.layer;
            go.transform.parent = parent; go.transform.localPosition = position; go.transform.localScale = Vector3.one;
            UITexture value = go.AddComponent<UITexture>(); value.mainTexture = Texture2D.whiteTexture;
            value.width = width; value.height = height; value.depth = depth; value.color = color; return value;
        }
        void MakeRemember()
        {
            GameObject go = new GameObject("RememberPassword"); go.layer = register.gameObject.layer;
            go.transform.parent = register.transform.parent; go.transform.localScale = register.transform.localScale;
            go.transform.localPosition = register.transform.localPosition + new Vector3(-75f, 64f, 0f);
            int depth = register.GetComponent<UISprite>().depth + 10;
            UITexture border = go.AddComponent<UITexture>(); border.mainTexture = Texture2D.whiteTexture;
            border.width = 21; border.height = 21; border.depth = depth;
            border.color = new Color(0.49f, 0.65f, 0.71f, 1f);
            rememberFace = Solid(go.transform, "Face", 17, 17, Vector3.zero, depth + 1, Color.white);
            rememberTick = new GameObject("Checkmark"); rememberTick.layer = go.layer;
            rememberTick.transform.parent = go.transform; rememberTick.transform.localPosition = Vector3.zero; rememberTick.transform.localScale = Vector3.one;
            UITexture a = Solid(rememberTick.transform, "ShortStroke", 7, 3, new Vector3(-3f, -1f, 0f), depth + 2, Color.white);
            a.transform.localRotation = Quaternion.Euler(0f, 0f, -45f);
            UITexture b = Solid(rememberTick.transform, "LongStroke", 12, 3, new Vector3(3f, 1f, 0f), depth + 2, Color.white);
            b.transform.localRotation = Quaternion.Euler(0f, 0f, 45f);
            GameObject caption = new GameObject("Caption"); caption.layer = go.layer;
            caption.transform.parent = go.transform; caption.transform.localPosition = new Vector3(18f, 0f, 0f); caption.transform.localScale = Vector3.one;
            rememberLabel = caption.AddComponent<UILabel>(); UILabel theme = owner.nameInput.GetComponent<UIInput>().label;
            if (theme.bitmapFont != null) rememberLabel.bitmapFont = theme.bitmapFont; else rememberLabel.trueTypeFont = theme.trueTypeFont;
            rememberLabel.material = theme.material; rememberLabel.fontSize = 17; rememberLabel.color = Ink;
            rememberLabel.width = 120; rememberLabel.height = 28; rememberLabel.pivot = UIWidget.Pivot.Left;
            rememberLabel.alignment = NGUIText.Alignment.Left; rememberLabel.depth = depth + 2;
            rememberLabel.effectStyle = UILabel.Effect.None; rememberLabel.text = "记住密码";
            caption.transform.localPosition = new Vector3(18f, 0f, 0f);
            BoxCollider hit = go.AddComponent<BoxCollider>(); hit.center = new Vector3(55f, 0f, 0f); hit.size = new Vector3(150f, 34f, 1f);
            remember = go.AddComponent<UIButton>(); remember.tweenTarget = border.gameObject; remember.defaultColor = border.color;
            remember.hover = new Color(0.35f, 0.74f, 0.82f, 1f); remember.pressed = new Color(0.2f, 0.65f, 0.75f, 1f);
            EventDelegate.Add(remember.onClick, ChangeRemember);
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
                // A native checkbox and label sit between the password row and the two actions.
                UIButton unused = Find("OnForgetPassWord");
                if (unused != null) unused.gameObject.SetActive(false);
                unused = Find("OnTouristsLogin"); if (unused != null) unused.gameObject.SetActive(false);
                if (register != null)
                {
                    MakeRemember();
                }
                GameObject confirm = (GameObject)UnityEngine.Object.Instantiate(input.passWordInput);
                confirm.name = "ConfirmPassword"; confirm.transform.parent = input.passWordInput.transform.parent;
                confirm.transform.localScale = input.passWordInput.transform.localScale;
                confirm.transform.localPosition = passwordPosition + Vector3.down * 52f;
                // Cloning the password field also clones its localization and saved-value bindings.
                // The confirmation field owns its placeholder and never loads/stores a password preference.
                foreach (UILocalize localized in confirm.GetComponentsInChildren<UILocalize>(true)) localized.enabled = false;
                confirmation = confirm.GetComponent<UIInput>(); confirmation.value = "";
                confirmation.savedAs = String.Empty;
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
            ApplyTitle();
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
            StartCoroutine(RefreshPlaceholders());
        }
        void Apply()
        {
            // Fit the extra confirmation row in the same original panel; the title scene stays intact.
            owner.nameInput.transform.localPosition = namePosition;
            owner.passWordInput.transform.localPosition = passwordPosition;
            if (confirmation != null)
            {
                confirmation.transform.localPosition = passwordPosition + Vector3.down * 52f;
                if (registering && !confirmation.gameObject.activeSelf && String.IsNullOrEmpty(confirmation.value))
                {
                    confirmation.defaultText = "确认密码：请再次输入";
                    confirmation.label.text = confirmation.defaultText;
                }
                confirmation.gameObject.SetActive(registering);
                confirmation.UpdateLabel();
            }
            ApplyTitle();
            if (loginLabel != null) loginLabel.text = busy ? "注册中…" : registering ? "注册" : "登录";
            if (registerLabel != null) registerLabel.text = registering ? "返回登录" : "注册账号";
            if (login != null) login.isEnabled = !busy;
            if (register != null) register.isEnabled = !busy;
            if (remember != null)
            {
                remember.gameObject.SetActive(!registering);
                bool selected;
                using (AndroidJavaClass cache = Cache()) selected = cache.CallStatic<bool>("rememberEnabled");
                if (rememberFace != null) rememberFace.color = selected ? new Color(0.16f, 0.71f, 0.81f, 1f) : new Color(0.96f, 0.98f, 0.99f, 1f);
                if (rememberTick != null) rememberTick.SetActive(selected);
            }
        }
        void ApplyTitle()
        {
            // The shipped header is a sibling of UserInput. Stop at the nearest
            // ancestor containing its caption, preserving unrelated scene labels.
            for (Transform node = owner.transform; node != null; node = node.parent)
            {
                bool found = false;
                foreach (UILabel title in node.GetComponentsInChildren<UILabel>(true))
                if (title.text == "游戏登录" || title.text == "游戏注册" || title.text == "Brain Burst 2039" || title.text == "注册 Brain Burst 2039")
                {
                    foreach (UILocalize localized in title.GetComponents<UILocalize>()) localized.enabled = false;
                    title.text = registering ? "注册 Brain Burst 2039" : "Brain Burst 2039";
                    title.fontSize = 20;
                    title.width = Math.Max(title.width, 310);
                    title.overflowMethod = UILabel.Overflow.ShrinkContent;
                    found = true;
                }
                if (found) return;
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
