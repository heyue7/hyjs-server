package com.hyjs.wallet;

import android.content.Context;
import org.json.JSONArray;
import org.json.JSONObject;
import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.HashSet;
import java.util.List;

/** Refresh only the six approved reproducible resource-cache files, before Unity starts. */
public final class LocalAssetOverlay {
    private LocalAssetOverlay() { }
    private static final String CONFIG = "local-resource-overrides.json";
    private static final Pin[] PINS = {
        new Pin("tabledata/tablehero", "f45ffdffbd92df69ebae0b549002a3adeb4440c716bcc9145af68b121e757076", 10169L),
        new Pin("tabledata/tablefrag", "f489c4f769aac38633111f7a1c4bb0ed4164f16d65df01adc03524ea07210512", 3253L),
        new Pin("model/lingnai01_body", "0bb4db13e9c6667b6b52d88a1a469990895a154b6c38509dbdfe64a424322d4e", 526777L),
        new Pin("model/sicary01_body", "1dc1308c407872e7ad009ac97abae56710026e1a3572b043389597d135483d77", 596264L),
        new Pin("model/nio01_body", "5aa707be0d98bf19082fb05fcac5a10029cafca95a90aa9905ddfa38eb6e647a", 503359L),
        new Pin("smallassetmanifest", "cfddb4d1b6ee15bfc8d51da311bfa7e6e1408c809c4ccd8b1e47049a85b45254", 563672L),
    };
    private static final class Pin {
        final String path, sha256;
        final long bytes;
        Pin(String p, String s, long b) { path=p; sha256=s; bytes=b; }
    }
    private static final class Staged {
        final File temporary, target;
        Staged(File a,File b) { temporary=a;target=b; }
    }

    /** Invoke before super.onCreate; IOException means Unity must not start on mixed resources. */
    public static synchronized void synchronize(Context context) throws IOException {
        if (context == null) throw new IOException("Resource overlay context unavailable");
        verifyConfiguration(context);
        // Unity builds may choose internal or external persistentDataPath.
        // Refresh the same six explicit cache paths at both roots before startup.
        File internal = context.getFilesDir();
        if (internal == null) throw new IOException("Game resource storage unavailable");
        synchronizeDirectory(context, internal);
        File external = context.getExternalFilesDir(null);
        if (external != null && !external.getCanonicalFile().equals(internal.getCanonicalFile()))
            synchronizeDirectory(context, external);
    }

    private static void synchronizeDirectory(Context context, File directory) throws IOException {
        File root=directory.getCanonicalFile();
        if ((!root.isDirectory() && !root.mkdirs()) || !root.isDirectory())
            throw new IOException("Game resource storage unavailable");
        List<Staged> staged=new ArrayList<Staged>();
        try {
            // Verify and stage every changed APK resource before replacing any live file.
            for (Pin pin:PINS) {
                File intended=new File(root,pin.path).getAbsoluteFile();
                File target=intended.getCanonicalFile();
                if (!target.getPath().equals(intended.getPath()) || !target.getPath().startsWith(root.getPath()+File.separator))
                    throw new IOException("Resource cache path rejected");
                if (matches(target,pin)) continue;
                File parent=target.getParentFile();
                if ((!parent.isDirectory() && !parent.mkdirs()) || !parent.isDirectory())
                    throw new IOException("Resource cache directory unavailable");
                File temporary=File.createTempFile(".hyjs-green-overlay-",".tmp",parent);
                staged.add(new Staged(temporary,target));
                MessageDigest digest=digest();
                long size=0;
                try (InputStream source=context.getAssets().open("android/"+pin.path);
                     FileOutputStream output=new FileOutputStream(temporary)) {
                    byte[] buffer=new byte[32768]; int count;
                    while ((count=source.read(buffer))!=-1) {
                        size+=count;
                        if (size>pin.bytes) throw new IOException("APK resource size mismatch: "+pin.path);
                        digest.update(buffer,0,count);output.write(buffer,0,count);
                    }
                    output.flush();output.getFD().sync();
                }
                if (size!=pin.bytes || !hex(digest.digest()).equals(pin.sha256))
                    throw new IOException("APK resource checksum mismatch: "+pin.path);
            }
            // PINS is ordered: five payload bundles first; smallassetmanifest commits last.
            // A crash is safe to retry: completed payload hashes are recognized next launch.
            for (Staged item:staged) {
                if (!item.temporary.renameTo(item.target))
                    throw new IOException("Atomic resource replacement failed");
            }
        } finally {
            for (Staged item:staged) if (item.temporary.exists()) item.temporary.delete();
        }
    }

    private static void verifyConfiguration(Context context) throws IOException {
        byte[] encoded;
        try (InputStream input=context.getAssets().open(CONFIG);
             ByteArrayOutputStream output=new ByteArrayOutputStream()) {
            byte[] buffer=new byte[4096];int count;
            while ((count=input.read(buffer))!=-1) {
                if (output.size()+count>16384) throw new IOException("Resource configuration too large");
                output.write(buffer,0,count);
            }
            encoded=output.toByteArray();
        }
        try {
            JSONObject root=new JSONObject(new String(encoded,"UTF-8"));
            if (!root.getString("schema").equals("hyjs-green-resource-overrides-v1") || root.getInt("version")!=1)
                throw new IOException("Resource configuration version mismatch");
            JSONArray files=root.getJSONArray("files");
            if (files.length()!=PINS.length) throw new IOException("Resource whitelist length mismatch");
            HashSet<String> seen=new HashSet<String>();
            for (int i=0;i<PINS.length;i++) {
                JSONObject row=files.getJSONObject(i);Pin pin=PINS[i];
                if (!seen.add(row.getString("path")) || !row.getString("path").equals(pin.path)
                    || !row.getString("sha256").equals(pin.sha256) || row.getLong("bytes")!=pin.bytes)
                    throw new IOException("Resource configuration pin mismatch");
            }
        } catch (IOException error) { throw error; }
        catch (Exception error) { throw new IOException("Resource configuration unreadable",error); }
    }

    private static boolean matches(File file,Pin pin) throws IOException {
        if (!file.isFile() || file.length()!=pin.bytes) return false;
        MessageDigest hash=digest();long size=0;
        try (InputStream input=new FileInputStream(file)) {
            byte[] buffer=new byte[32768];int count;
            while ((count=input.read(buffer))!=-1) {size+=count;hash.update(buffer,0,count);}
        }
        return size==pin.bytes && hex(hash.digest()).equals(pin.sha256);
    }
    private static MessageDigest digest() throws IOException {
        try { return MessageDigest.getInstance("SHA-256"); }
        catch (NoSuchAlgorithmException error) {throw new IOException("SHA-256 unavailable",error);}
    }
    private static String hex(byte[] value) {
        char[] alphabet="0123456789abcdef".toCharArray();char[] text=new char[value.length*2];
        for (int i=0;i<value.length;i++) {int n=value[i]&255;text[i*2]=alphabet[n>>>4];text[i*2+1]=alphabet[n&15];}
        return new String(text);
    }
}
