package com.hyjs.wallet;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URI;
import java.net.URL;
import org.json.JSONObject;

/** A password-authenticated account phase, independent of entering a game realm. */
public final class GameAccountLogin {
    private static int serial, state;
    private static String error="", pendingToken="", pendingAccount="";
    private static HttpURLConnection active;
    private GameAccountLogin() { }

    public static int submitNative(final String serverBase,final String username,final String password){
        final int request;
        synchronized(GameAccountLogin.class){
            cancelLocked();request=++serial;state=0;error="";
            WalletActivity.bindAccountSession("","");WalletActivity.bindSession(0,"");
        }
        new Thread(()->authenticate(request,serverBase,username,password),"hyjs-account-login").start();
        return request;
    }
    public static synchronized int nativeStatus(int request){return request==serial?state:3;}
    public static synchronized String nativeError(int request){return request==serial?error:"登录请求已取消";}
    public static synchronized boolean acceptNative(int request,String username,String password){
        if(request!=serial || state!=1 || !pendingToken.matches("[a-f0-9]{64}"))return false;
        LoginCache.accountAuthenticated(username,password);
        WalletActivity.bindAccountSession(pendingToken,pendingAccount);
        pendingToken="";pendingAccount="";state=3;
        return true;
    }
    private static void cancelLocked(){
        ++serial;state=3;error="";pendingToken="";pendingAccount="";
        HttpURLConnection previous=active;active=null;if(previous!=null)previous.disconnect();
    }
    public static synchronized void cancelNative(){cancelLocked();}
    public static synchronized void clearAccount(){
        cancelLocked();WalletActivity.bindAccountSession("","");WalletActivity.bindSession(0,"");
    }
    private static void authenticate(int request,String serverBase,String username,String password){
        HttpURLConnection connection=null;
        String token="",account="",message="账号服务暂时不可用，请稍后重试";
        try{
            URI base=new URI(serverBase);
            if((!"http".equals(base.getScheme()) && !"https".equals(base.getScheme())) || base.getHost()==null || base.getUserInfo()!=null)throw new IllegalArgumentException();
            URL endpoint=new URI(base.getScheme(),null,base.getHost(),base.getPort(),"/game-account/login",null,null).toURL();
            connection=(HttpURLConnection)endpoint.openConnection();
            synchronized(GameAccountLogin.class){if(request!=serial)return;active=connection;}
            connection.setConnectTimeout(8000);connection.setReadTimeout(15000);
            connection.setRequestMethod("POST");connection.setDoOutput(true);connection.setInstanceFollowRedirects(false);
            connection.setRequestProperty("Content-Type","application/json; charset=utf-8");connection.setRequestProperty("Cache-Control","no-store");
            byte[] bytes=new JSONObject().put("username",username).put("password",password).toString().getBytes("UTF-8");
            connection.setFixedLengthStreamingMode(bytes.length);
            try(OutputStream stream=connection.getOutputStream()){stream.write(bytes);}
            int status=connection.getResponseCode();
            InputStream incoming=status>=400?connection.getErrorStream():connection.getInputStream();
            if(incoming==null)throw new IllegalArgumentException();
            ByteArrayOutputStream output=new ByteArrayOutputStream();
            try(InputStream stream=incoming){
                byte[] buffer=new byte[2048];int count;
                while((count=stream.read(buffer))!=-1){if(output.size()+count>262144)throw new IllegalArgumentException();output.write(buffer,0,count);}
            }
            JSONObject reply=new JSONObject(new String(output.toByteArray(),"UTF-8"));
            JSONObject data=reply.optJSONObject("data");JSONObject identity=data==null?null:data.optJSONObject("account");
            String received=data==null?"":data.optString("accountSession");
            if(status==200 && reply.optInt("code",-1)==0 && received.matches("[a-f0-9]{64}") && identity!=null && identity.optLong("id")>0 && username.equals(identity.optString("username"))){
                token=received;account=identity.toString();message="";
            }else{
                String reason=reply.optString("msg");if(!reason.isEmpty() && reason.length()<=240)message=reason;
            }
        }catch(Exception unavailable){/* Credentials and server response details are never logged. */}
        finally{if(connection!=null)connection.disconnect();}
        synchronized(GameAccountLogin.class){
            if(active==connection)active=null;
            if(request!=serial)return;
            pendingToken=token;pendingAccount=account;error=message;state=token.isEmpty()?2:1;
        }
    }
}
