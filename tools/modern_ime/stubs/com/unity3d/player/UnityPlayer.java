package com.unity3d.player;

import android.content.Context;
import android.widget.FrameLayout;

// Compile-only ABI stub. It must never be defined in the output dex.
public class UnityPlayer extends FrameLayout {
    public static android.app.Activity currentActivity;
    public UnityPlayer(Context context) { super(context); }
    protected void reportSoftInputStr(String text, int status, boolean canceled) { }
}
