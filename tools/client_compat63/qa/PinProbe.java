package com.hyjs.compat.qa;
import java.lang.reflect.Field;

/** Read-only ART class initializer probe. No Context, resource synchronization, Unity or login. */
public final class PinProbe {
    private static Object field(Object value, String name) throws Exception {
        Field field=value.getClass().getDeclaredField(name); field.setAccessible(true); return field.get(value);
    }
    public static void main(String[] ignored) {
        try {
            Class<?> overlay=Class.forName("com.hyjs.wallet.LocalAssetOverlay",true,PinProbe.class.getClassLoader());
            Field field=overlay.getDeclaredField("PINS");field.setAccessible(true);
            Object[] pins=(Object[])field.get(null);
            if(pins.length!=6) throw new IllegalStateException();
            String[] whitelist={"tabledata/tablehero","tabledata/tablefrag","model/lingnai01_body","model/sicary01_body","model/nio01_body","smallassetmanifest"};
            for(int i=0;i<pins.length;i++) {
                Object pin=pins[i];String path=(String)field(pin,"path");String sha=(String)field(pin,"sha256");long bytes=((Long)field(pin,"bytes")).longValue();
                if(!whitelist[i].equals(path) || !sha.matches("[0-9a-f]{64}") || bytes<0 || bytes>10000000)throw new IllegalStateException();
                System.out.println("HYJSPinProbe PIN "+path+" bytes="+bytes+" sha256="+sha);
            }
            System.out.println("HYJSPinProbe PASS initialized six actual ART pins; no synchronization performed");
        } catch(Throwable error) {
            for(int depth=0;error!=null && depth<4;depth++) {
                System.out.println("HYJSPinProbe FAIL exception["+depth+"]="+error.getClass().getName());
                if(error instanceof VerifyError)System.out.println("HYJSPinProbe VERIFY_MESSAGE="+error.getMessage());
                StackTraceElement[] frames=error.getStackTrace();
                for(int i=0;i<frames.length && i<16;i++)System.out.println(" at "+frames[i].getClassName()+"."+frames[i].getMethodName()+" line="+frames[i].getLineNumber());
                Throwable next=error.getCause();if(next==error)break;error=next;
            }
            System.exit(2);
        }
    }
}
