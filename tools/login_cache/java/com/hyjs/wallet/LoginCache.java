package com.hyjs.wallet;

import android.app.Activity;
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
    private static LinearLayout row;
    private static CheckBox rememberBox,autoBox;
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
            savedUser=value.getString("username");savedPassword=value.getString("password");remember=!savedUser.isEmpty()&&!savedPassword.isEmpty();automatic=remember&&value.optBoolean("auto");
        }catch(Exception unusable){clear();}
    }
    private static void clear(){savedUser="";savedPassword="";try{prefs().edit().clear().commit();}catch(Exception ignored){}}
    private static void save(){
        if(!remember){clear();return;}
        try{
            JSONObject value=new JSONObject().put("username",savedUser).put("password",savedPassword).put("auto",automatic);
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
    public static synchronized void boot(String object){load();loginOwner=object;if(automatic&&!suppressed&&!autoConsumed&&!bootQueued){bootQueued=true;UI.postDelayed(()->{synchronized(LoginCache.class){if(!suppressed&&!autoConsumed)send(loginOwner,"OnPressLogin");}},350);}}
    public static synchronized void show(String object){
        load();owner=object;final boolean shouldAuto=automatic&&!suppressed&&!autoConsumed;
        if(shouldAuto){autoConsumed=true;autoAttempt=true;UI.postDelayed(()->{synchronized(LoginCache.class){if(autoAttempt&&active==0)fail();}},20000);}
        UI.post(()->{
            hideNow();try{
                Activity a=activity();FrameLayout host=(FrameLayout)a.findViewById(android.R.id.content);
                row=new LinearLayout(a);row.setOrientation(LinearLayout.HORIZONTAL);row.setGravity(android.view.Gravity.CENTER);
                rememberBox=box(a,"记住密码");autoBox=box(a,"自动登录");rememberBox.setChecked(remember);autoBox.setChecked(automatic);row.addView(rememberBox);row.addView(autoBox);
                FrameLayout.LayoutParams layout=new FrameLayout.LayoutParams(Math.round(host.getWidth()*.34f),Math.round(host.getHeight()*.075f));layout.leftMargin=Math.round(host.getWidth()*.35f);layout.topMargin=Math.round(host.getHeight()*.43f);host.addView(row,layout);
                rememberBox.setOnCheckedChangeListener((button,checked)->{synchronized(LoginCache.class){remember=checked;if(!checked){automatic=false;autoBox.setChecked(false);clear();}}});
                autoBox.setOnCheckedChangeListener((button,checked)->{synchronized(LoginCache.class){automatic=checked;if(checked){remember=true;rememberBox.setChecked(true);}else if(autoAttempt){autoAttempt=false;suppressed=true;}if(!savedUser.isEmpty())save();}});
                if(shouldAuto)UI.postDelayed(()->{synchronized(LoginCache.class){if(!suppressed&&autoAttempt)send(owner,"OnLoginButton");}},500);
            }catch(Exception unavailable){fail();}
        });
    }
    private static CheckBox box(Activity a,String title){CheckBox result=new CheckBox(a);result.setText(title);result.setTextColor(Color.rgb(53,79,90));result.setTextSize(14);result.setScaleX(.70f);result.setScaleY(.70f);result.setButtonTintList(android.content.res.ColorStateList.valueOf(Color.rgb(28,123,163)));return result;}
    private static void hideNow(){if(row!=null&&row.getParent() instanceof ViewGroup)((ViewGroup)row.getParent()).removeView(row);row=null;}
    public static void hide(){UI.post(()->hideNow());}
    public static synchronized void ready(){if(autoAttempt&&!suppressed)UI.postDelayed(()->{synchronized(LoginCache.class){if(autoAttempt&&!suppressed)send(loginOwner,"OnPressLogin");}},100);}
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
