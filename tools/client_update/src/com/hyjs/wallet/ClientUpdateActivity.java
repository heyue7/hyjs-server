package com.hyjs.wallet;
import android.os.Bundle;
/** APK composition must remove only WalletActivity's ACC_FINAL class flag. */
public final class ClientUpdateActivity extends WalletActivity {
    @Override protected void onCreate(Bundle state){super.onCreate(state);ClientUpdate.boot(this);}
    @Override protected void onResume(){super.onResume();ClientUpdate.resume();}
    @Override protected void onPause(){ClientUpdate.pause();super.onPause();}
}
