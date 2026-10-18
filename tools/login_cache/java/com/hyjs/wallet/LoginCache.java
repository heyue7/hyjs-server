package com.hyjs.wallet;

import android.app.Activity;
import android.app.Dialog;
import android.content.DialogInterface;
import android.text.Editable;
import android.text.TextWatcher;
import android.view.View;
import android.view.inputmethod.EditorInfo;
import android.widget.Button;
import android.widget.EditText;
import android.widget.TextView;
import android.content.SharedPreferences;
import android.graphics.Color;
import android.os.Handler;
import android.os.Looper;
import android.security.keystore.KeyGenParameterSpec;
import android.security.keystore.KeyProperties;
import android.util.Base64;
import android.view.ViewGroup;
import android.widget.CheckBox;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.Toast;
import java.security.KeyStore;
import java.util.HashSet;
import javax.crypto.Cipher;
import javax.crypto.KeyGenerator;
import javax.crypto.SecretKey;
import javax.crypto.spec.GCMParameterSpec;
import org.json.JSONObject;

/** Credentials are committed only after an accepted, successful original login. */
public final class LoginCache {
    private static final String ALIAS="hyjs.remember-login.v1", PREF="hyjs-encrypted-login-v1";
    private static final Handler UI=new Handler(Looper.getMainLooper());
    private static String savedUser="",savedPassword="",pendingUser="",pendingPassword="",owner="",loginOwner="loginUI";
    private static boolean loaded,remember,automatic,suppressed,autoConsumed,autoAttempt,bootQueued;
    private static int serial,active;
    private static final HashSet<Integer> used=new HashSet<Integer>();
    private static String transientUser="",transientPassword="";
    private static Dialog dialog;
    private static LinearLayout row;
    private static boolean formVisible;
    private static String formUser="",formPass="",returnUser="",returnPass="";
    private static CheckBox rememberBox;
    private static Activity activity()throws Exception{return (Activity)Class.forName("com.unity3d.player.UnityPlayer").getField("currentActivity").get(null);}
    private static void send(String object,String method){try{Class.forName("com.unity3d.player.UnityPlayer").getMethod("UnitySendMessage",String.class,String.class,String.class).invoke(null,object,method,"");}catch(Exception ignored){}}
    private static SecretKey key()throws Exception{
        KeyStore store=KeyStore.getInstance("AndroidKeyStore");store.load(null);
        if(!store.containsAlias(ALIAS)){
            KeyGenerator generator=KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES,"AndroidKeyStore");
            generator.init(new KeyGenParameterSpec.Builder(ALIAS,KeyProperties.PURPOSE_ENCRYPT|KeyProperties.PURPOSE_DECRYPT).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).setRandomizedEncryptionRequired(true).build());generator.generateKey();
        }
        return (SecretKey)store.getKey(ALIAS,null);
    }
    private static SharedPreferences prefs()throws Exception{return activity().getSharedPreferences(PREF,0);}
    private static synchronized void load(){
        if(loaded)return;loaded=true;
        try{
            SharedPreferences preferences=prefs();String data=preferences.getString("ciphertext",null),iv=preferences.getString("iv",null);
            if(data==null||iv==null)return;
            Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.DECRYPT_MODE,key(),new GCMParameterSpec(128,Base64.decode(iv,Base64.NO_WRAP)));
            JSONObject value=new JSONObject(new String(cipher.doFinal(Base64.decode(data,Base64.NO_WRAP)),"UTF-8"));
            savedUser=value.getString("username");savedPassword=value.getString("password");remember=!savedUser.isEmpty()&&!savedPassword.isEmpty();automatic=false;
        }catch(Exception unusable){clear();}
    }
    private static void clear(){savedUser="";savedPassword="";try{prefs().edit().clear().commit();}catch(Exception ignored){}}
    private static void save(){
        if(!remember){clear();return;}
        try{
            JSONObject value=new JSONObject().put("username",savedUser).put("password",savedPassword).put("auto",false);
            Cipher cipher=Cipher.getInstance("AES/GCM/NoPadding");cipher.init(Cipher.ENCRYPT_MODE,key());
            byte[] encrypted=cipher.doFinal(value.toString().getBytes("UTF-8"));
            if(!prefs().edit().putString("ciphertext",Base64.encodeToString(encrypted,Base64.NO_WRAP)).putString("iv",Base64.encodeToString(cipher.getIV(),Base64.NO_WRAP)).commit())throw new Exception();
        }catch(Exception failed){clear();remember=false;automatic=false;toast("密码未能加密保存，请手动登录");}
    }
    public static synchronized String username(){load();return remember?savedUser:"";}
    public static synchronized String password(){load();return remember?savedPassword:"";}
    public static synchronized String transientUserName(){return transientUser;}
    public static synchronized String transientPassWord(){return transientPassword;}
    public static synchronized void setTransientUserName(String value){transientUser=value==null?"":value;}
    public static synchronized void setTransientPassWord(String value){transientPassword=value==null?"":value;}
    public static synchronized void boot(String object){load();loginOwner=object;automatic=false;autoAttempt=false;suppressed=true;}
    public static synchronized boolean hasForm(){return formVisible;}
    public static synchronized String formUserName(){return formUser;}
    public static synchronized String formPassWord(){return formPass;}
    private static synchronized void updateForm(EditText username,EditText password){
        formUser=username.getText().toString();formPass=password.getText().toString();
    }
    private static synchronized void manual(){autoAttempt=false;suppressed=true;}
    public static void reopen(){String target;synchronized(LoginCache.class){target=owner;}if(target!=null&&!target.isEmpty())show(target);}
    public static void registered(String username,String password){synchronized(LoginCache.class){returnUser=username;returnPass=password;}reopen();}
    public static synchronized void show(String object){
        load();owner=object;final String target=object;
        automatic=false;autoAttempt=false;
        UI.post(()->{
            hideNow();try{
                Activity a=activity();if(a==null||a.isFinishing())return;
                final Dialog opened=new Dialog(a);opened.requestWindowFeature(android.view.Window.FEATURE_NO_TITLE);
                final LinearLayout body=LoginStyle.body(a,"账号登录");
                final EditText username=LoginStyle.field(a,body,"请输入账号",false);
                final EditText password=LoginStyle.field(a,body,"请输入密码",true);
                password.setImeOptions(EditorInfo.IME_ACTION_DONE|EditorInfo.IME_FLAG_NO_EXTRACT_UI);
                synchronized(LoginCache.class){
                    username.setText(!returnUser.isEmpty()?returnUser:(remember?savedUser:""));
                    password.setText(!returnUser.isEmpty()?returnPass:(remember?savedPassword:""));
                    returnUser="";returnPass="";updateForm(username,password);
                }
                TextWatcher watcher=new TextWatcher(){
                    public void beforeTextChanged(CharSequence text,int start,int count,int after){}
                    public void onTextChanged(CharSequence text,int start,int before,int count){synchronized(LoginCache.class){if(dialog==opened)updateForm(username,password);}}
                    public void afterTextChanged(Editable text){}
                };
                username.addTextChangedListener(watcher);password.addTextChangedListener(watcher);
                row=new LinearLayout(a);row.setOrientation(LinearLayout.HORIZONTAL);row.setGravity(android.view.Gravity.CENTER_VERTICAL);
                rememberBox=box(a,"记住密码");
                rememberBox.setChecked(remember);
                row.addView(rememberBox,new LinearLayout.LayoutParams(0,LoginStyle.dp(a,46),1f));
                body.addView(row,new LinearLayout.LayoutParams(-1,LoginStyle.dp(a,48)));
                rememberBox.setOnCheckedChangeListener((button,checked)->{synchronized(LoginCache.class){remember=checked;if(!checked){automatic=false;clear();}}});
                Button login=LoginStyle.button(a,"登 录",true);
                LinearLayout.LayoutParams loginLayout=new LinearLayout.LayoutParams(-1,LoginStyle.dp(a,44));loginLayout.topMargin=LoginStyle.dp(a,4);body.addView(login,loginLayout);
                LinearLayout actions=new LinearLayout(a);actions.setOrientation(LinearLayout.HORIZONTAL);
                Button close=LoginStyle.button(a,"返回",false),register=LoginStyle.button(a,"注册账号",false);
                LinearLayout.LayoutParams actionLayout=new LinearLayout.LayoutParams(0,LoginStyle.dp(a,38),1f);actionLayout.topMargin=LoginStyle.dp(a,8);
                actions.addView(close,actionLayout);LinearLayout.LayoutParams registerLayout=new LinearLayout.LayoutParams(0,LoginStyle.dp(a,38),1f);registerLayout.topMargin=LoginStyle.dp(a,8);registerLayout.leftMargin=LoginStyle.dp(a,12);actions.addView(register,registerLayout);
                body.addView(actions,new LinearLayout.LayoutParams(-1,-2));
                login.setOnClickListener(view->{manual();send(target,"OnLoginButton");});
                password.setOnEditorActionListener((text,action,event)->{if(action==EditorInfo.IME_ACTION_DONE){manual();send(target,"OnLoginButton");return true;}return false;});
                register.setOnClickListener(view->{manual();send(target,"OnRegistButtonClick");});
                close.setOnClickListener(view->{manual();send(target,"OnCloseButtonClick");});
                opened.setContentView(LoginStyle.panel(a,body));opened.setCanceledOnTouchOutside(false);
                opened.setOnCancelListener(ignored->{manual();send(target,"OnCloseButtonClick");});
                opened.setOnDismissListener(ignored->{synchronized(LoginCache.class){if(dialog==opened){dialog=null;formVisible=false;formUser="";formPass="";row=null;rememberBox=null;}}username.setText("");password.setText("");});
                synchronized(LoginCache.class){dialog=opened;formVisible=true;}
                opened.show();LoginStyle.fit(a,opened);body.setFocusableInTouchMode(true);body.requestFocus();
            }catch(Exception unavailable){fail();}
        });
    }
    private static CheckBox box(Activity a,String title){CheckBox result=new CheckBox(a);result.setText(title);result.setTextColor(LoginStyle.INK);result.setTextSize(15);result.setPadding(LoginStyle.dp(a,2),0,LoginStyle.dp(a,8),0);result.setButtonTintList(android.content.res.ColorStateList.valueOf(LoginStyle.BLUE));return result;}
    private static void hideNow(){Dialog previous;synchronized(LoginCache.class){previous=dialog;dialog=null;formVisible=false;formUser="";formPass="";row=null;rememberBox=null;}if(previous!=null)previous.dismiss();}
    public static void hide(){UI.post(()->hideNow());}
    public static synchronized void ready(){/* Login always waits for an explicit user action. */}
    public static synchronized boolean begin(int identity,String username,String password){
        if(active!=0)return false;
        if(identity==0||!used.add(identity)){fail();return false;}
        pendingUser=username;pendingPassword=password;active=identity;final int attempt=active;hide();
        UI.postDelayed(()->{synchronized(LoginCache.class){if(active==attempt){active=0;pendingUser="";pendingPassword="";fail();}}},20000);
        return true;
    }
    public static synchronized boolean accepts(int attempt){return attempt!=0&&attempt==active;}
    public static synchronized void complete(int attempt,boolean success){
        if(attempt==0||attempt!=active)return;
        active=0;
        if(success){if(remember){savedUser=pendingUser;savedPassword=pendingPassword;save();}else clear();autoAttempt=false;}
        else fail();
        pendingUser="";pendingPassword="";
    }
    private static void toast(String message){UI.post(()->{try{Toast.makeText(activity(),message,Toast.LENGTH_SHORT).show();}catch(Exception ignored){}});}
    private static void fail(){boolean wasAutomatic=autoAttempt;autoAttempt=false;suppressed=true;hide();if(wasAutomatic)toast("自动登录未完成，请手动登录");UI.post(()->send(loginOwner,"OnSwitchAccountButtonClick"));}
    public static synchronized void pause(){suppressed=true;autoAttempt=false;active=0;++serial;pendingUser="";pendingPassword="";hide();}
}
