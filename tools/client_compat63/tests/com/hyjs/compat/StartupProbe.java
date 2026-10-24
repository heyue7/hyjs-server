package com.hyjs.compat;
import android.content.Context;
import android.content.res.AssetManager;
import com.hyjs.wallet.LocalAssetOverlay;
import java.io.*;
import java.nio.file.*;
import java.util.concurrent.CountDownLatch;

public final class StartupProbe {
 static void require(boolean ok,String message) { if (!ok) throw new AssertionError(message); }
 static final class FakeContext extends Context {
  final File assets,root; String fault="";
  FakeContext(File a,File r) { assets=a; root=r; }
  public File getFilesDir() { return new File(root,"internal"); }
  public File getExternalFilesDir(String ignored) { return new File(root,"external"); }
  public AssetManager getAssets() { return new AssetManager() {
   public InputStream open(String path) throws IOException {
    if (path.equals("android/tabledata/tablehero")) {
     if (fault.equals("io")) throw new IOException("PRIVATE PATH AND SECRET MUST NOT REACH UI");
     if (fault.equals("hash")) return new ByteArrayInputStream(new byte[10169]);
     if (fault.equals("permission")) throw new SecurityException("private-storage");
    }
    return new FileInputStream(new File(assets,path));
   }
  }; }
 }
 static void attempt(StartupGate gate,final FakeContext context,final int[] launches) throws Exception {
  if (!gate.begin()) return;
  gate.prepare(new StartupGate.Action(){public void run() throws Exception { LocalAssetOverlay.synchronize(context); }});
  gate.launch(new StartupGate.Action(){public void run(){ launches[0]++; }});
 }
 public static void main(String[] args) throws Exception {
  File assets=new File(args[0]), root=new File(args[1]); root.mkdirs();
  if (args.length>2 && args[2].equals("original")) {
   FakeContext formal=new FakeContext(assets,new File(root,"formal62-fresh"));
   StartupGate formalGate=new StartupGate(); int[] formalLaunches={0}; boolean formalFailed=false;
   try { attempt(formalGate,formal,formalLaunches); }
   catch(IOException expected) { formalFailed=expected.getMessage().equals("APK resource size mismatch: smallassetmanifest"); }
   require(formalFailed && formalLaunches[0]==0,"formal62 mismatch must fail closed");
   System.out.println("EXPECTED_BLOCKER formal62 fresh cache: smallassetmanifest pin/asset mismatch; zero Unity launches"); return;
  }
  int cases=0;
  for (String fault:new String[]{"hash","io","permission"}) {
   final FakeContext context=new FakeContext(assets,new File(root,fault));
   File files=context.getFilesDir(); files.mkdirs();
   Path sentinel=new File(files,"unrelated-save.keep").toPath(); Files.write(sentinel,new byte[]{42});
   StartupGate gate=new StartupGate(); int[] launches={0}; context.fault=fault; boolean failed=false;
   try { attempt(gate,context,launches); } catch (IOException|SecurityException expected) { failed=true; }
   require(failed && launches[0]==0 && !gate.launched(),fault+": failure started Unity");
   require(!new File(files,"tabledata/tablehero").exists(),fault+": failed stage committed");
   require(Files.readAllBytes(sentinel)[0]==42,"unrelated file changed");
   context.fault=""; attempt(gate,context,launches); attempt(gate,context,launches);
   require(launches[0]==1 && gate.launched(),fault+": retry did not launch exactly once");
   require(new File(context.getExternalFilesDir(null),"smallassetmanifest").isFile(),"external sync missing");
   cases++;
  }
  StartupGate pending=new StartupGate(); require(pending.begin(),"first begin"); require(!pending.begin(),"duplicate in-flight");
  pending.prepare(new StartupGate.Action(){public void run(){}}); pending.cancel();
  require(!pending.launch(new StartupGate.Action(){public void run(){throw new AssertionError("destroyed launch");}}),"cancelled launched"); cases++;
  StartupGate launchFailure=new StartupGate(); require(launchFailure.begin(),"begin");
  launchFailure.prepare(new StartupGate.Action(){public void run(){}});
  try {launchFailure.launch(new StartupGate.Action(){public void run() throws Exception {throw new IOException("startActivity failure");}});} catch(IOException expected){}
  require(!launchFailure.launched() && launchFailure.begin(),"handoff failure not retryable"); cases++;
  System.out.println("PASS "+cases+" cases: actual overlay hash/IO/security fail closed; retry exactly once; in-flight/cancel/handoff guards; unrelated file preserved");
 }
}
