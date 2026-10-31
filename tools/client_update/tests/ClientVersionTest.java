package com.hyjs.wallet;

import android.app.Activity;
import android.content.pm.PackageInfo;
import android.content.pm.PackageManager;
import android.os.Build;
import com.unity3d.player.UnityPlayer;
import java.io.*;
import java.lang.reflect.*;
import java.net.*;
import java.util.concurrent.*;
import org.json.JSONObject;

/** JVM exercise of the production bridge with in-memory Android and HTTP substitutes. */
public final class ClientVersionTest {
    private static final BlockingQueue<String> requests=new LinkedBlockingQueue<String>();
    private static final String SESSION="0123456789abcdef0123456789abcdef";
    private static void check(boolean value,String message){if(!value)throw new AssertionError(message);}
    private static Field field(String name)throws Exception{Field f=ClientUpdate.class.getDeclaredField(name);f.setAccessible(true);return f;}
    private static Method method(String name,Class<?>... types)throws Exception{Method m=ClientUpdate.class.getDeclaredMethod(name,types);m.setAccessible(true);return m;}
    private static Activity activity(long build,String name){
        PackageInfo info=new PackageInfo();info.versionCode=(int)build;info.longVersionCode=build;info.versionName=name;
        return new Activity(new PackageManager(info));
    }
    private static void reset(Activity activity)throws Exception{field("clientVersion").set(null,null);field("activity").set(null,activity);}
    private static void valid(String value){check(new ClientVersion(5,value).name.equals(value),"expected valid: "+value);}
    private static void invalid(String value){check(new ClientVersion(5,value).name.isEmpty(),"expected invalid: "+value);}
    private static void testParsingAndComparison(){
        for(String value:new String[]{"0.0.0","1.1.0","1.10.0","999.999.999"})valid(value);
        for(String value:new String[]{null,"","1","1.2","1.2.3.4","1..0",".1.0","1.0.","01.0.0","0.00.0","0.0.000","1000.0.0","-1.0.0","+1.0.0"," 1.0.0","1.0.0 ","1.\u0661.0","1.0.0-beta"})invalid(value);
        ClientVersion ten=new ClientVersion(900,"1.10.0");
        check(!ten.noticeEligible(true,false,901,"1.9.999"),"numeric ordering 1.10.0 > 1.9.999");
        check(!ten.noticeEligible(true,false,901,"1.10.0"),"equal version");
        check(ten.noticeEligible(true,false,1,"1.10.1"),"patch increase");
        check(ten.noticeEligible(true,false,1,"2.0.0"),"major increase");
        check(!ten.noticeEligible(true,false,901,"bad"),"invalid target must not look newer");
        check(ten.noticeEligible(true,true,1,"0.0.0"),"maintenance covers current versions");
        check(!ten.noticeEligible(false,true,900,"999.999.999"),"legacy equal build including maintenance");
        check(ten.noticeEligible(false,false,901,"0.0.0"),"legacy greater build");
        check(new ClientVersion(900,"bad").noticeEligible(true,false,1,"1.0.0"),"unknown relies on server notice");
    }
    private static void testFrozenIdentity()throws Exception{
        Activity first=activity(2026110101L,"1.10.0");reset(first);
        String login=ClientUpdate.version();String[] fields=login.split("/");
        check(fields.length==4 && fields[1].equals("2026110101") && fields[3].equals("1.10.0"),"login identity");
        JSONObject payload=(JSONObject)method("payload",Object.class).invoke(null,WalletActivity.credentials);
        check(payload.getLong("buildCode")==2026110101L && payload.getString("startupId").equals(fields[2]),"payload build and startup");
        check(payload.getString("clientVersion").equals("1.10.0"),"payload version");
        check(payload.getLong("playerId")==42 && payload.getString("sessionHex").equals(SESSION),"payload session");
        field("activity").set(null,activity(2026110102L,"2.0.0"));
        check(ClientUpdate.version().equals(login),"activity replacement changed login identity");
        check(((JSONObject)method("payload",Object.class).invoke(null,WalletActivity.credentials)).getString("clientVersion").equals("1.10.0"),"activity replacement changed payload");

        reset(activity(2026110103L,"01.1.0"));
        check(ClientUpdate.version().split("/").length==3,"invalid version must use old login format");
        payload=(JSONObject)method("payload",Object.class).invoke(null,WalletActivity.credentials);
        check(payload.getLong("buildCode")==2026110103L && !payload.has("clientVersion"),"invalid version payload");

        Activity missing=activity(1,"1.2.3");missing.getPackageManager().fail=true;reset(missing);
        check(ClientUpdate.version().startsWith("hyjs/0/"),"lookup failure remains unknown");
        missing.getPackageManager().fail=false;
        check(ClientUpdate.version().startsWith("hyjs/0/"),"failed lookup must remain frozen");
        check(!((JSONObject)method("payload",Object.class).invoke(null,WalletActivity.credentials)).has("clientVersion"),"failed lookup has no fake version");

        reset(null);UnityPlayer.currentActivity=activity(2026110104L,"1.2.3");
        check(ClientUpdate.version().endsWith("/1.2.3"),"Unity activity must cover early login");
        UnityPlayer.currentActivity=null;reset(null);
        check(ClientUpdate.version().split("/").length==3,"missing early activity must be unknown");
        field("activity").set(null,activity(2026110105L,"1.2.4"));
        check(ClientUpdate.version().startsWith("hyjs/0/"),"early unknown identity must stay consistent");

        Build.VERSION.SDK_INT=27;reset(activity(2026110106L,"1.2.5"));
        check(ClientUpdate.version().startsWith("hyjs/2026110106/"),"legacy Android versionCode");
        Build.VERSION.SDK_INT=33;
    }
    private static void testStatusAndAck()throws Exception{
        URL.setURLStreamHandlerFactory(protocol->"http".equals(protocol)?new URLStreamHandler(){
            @Override protected URLConnection openConnection(URL url){return new HttpURLConnection(url){
                final ByteArrayOutputStream body=new ByteArrayOutputStream();
                @Override public void disconnect(){} @Override public boolean usingProxy(){return false;} @Override public void connect(){}
                @Override public OutputStream getOutputStream(){return body;}
                @Override public int getResponseCode(){requests.add(url.getPath()+" "+body.toString());return 200;}
                @Override public InputStream getInputStream(){return new ByteArrayInputStream("{\"code\":0,\"data\":{}}".getBytes());}
            };}
        }:null);
        reset(activity(2026110107L,"1.2.6"));
        String[] login=ClientUpdate.version().split("/");
        field("origin").set(null,"http://test.invalid");field("accepted").setBoolean(null,true);field("active").setBoolean(null,true);
        method("check").invoke(null);
        String status=requests.poll(5,TimeUnit.SECONDS);
        check(status!=null && status.startsWith("/client-update/status "),"status request");
        method("ack",JSONObject.class,String.class,Object.class).invoke(null,new JSONObject().put("batchId","7"),"shown",WalletActivity.credentials);
        String ack=requests.poll(5,TimeUnit.SECONDS);
        check(ack!=null && ack.startsWith("/client-update/ack "),"ack request");
        for(String request:new String[]{status,ack}){
            check(request.contains("\"buildCode\":2026110107") && request.contains("\"startupId\":\""+login[2]+"\""),"request startup/build binding");
            check(request.contains("\"clientVersion\":\"1.2.6\"") && request.contains("\"sessionHex\":\""+SESSION+"\""),"request version/session binding");
        }
        check(ack.contains("\"kind\":\"shown\"") && ack.contains("\"batchId\":\"7\""),"ack metadata");

        reset(activity(2026110108L,"1.02.0"));
        login=ClientUpdate.version().split("/");
        check(login.length==3,"invalid version legacy login");
        method("check").invoke(null);
        status=requests.poll(5,TimeUnit.SECONDS);
        method("ack",JSONObject.class,String.class,Object.class).invoke(null,new JSONObject().put("batchId","8"),"shown",WalletActivity.credentials);
        ack=requests.poll(5,TimeUnit.SECONDS);
        for(String request:new String[]{status,ack}){
            check(request!=null && request.contains("\"buildCode\":2026110108") && request.contains("\"startupId\":\""+login[2]+"\""),"unknown request startup/build binding");
            check(!request.contains("clientVersion"),"unknown request must omit clientVersion");
        }
    }
    public static void main(String[] args)throws Exception{
        try{testParsingAndComparison();testFrozenIdentity();testStatusAndAck();System.out.println("ClientVersionTest passed");}
        finally{((ExecutorService)field("IO").get(null)).shutdownNow();}
    }
}
