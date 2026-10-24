package android.content;
import java.io.File;
import android.content.res.AssetManager;
public abstract class Context {
 public abstract File getFilesDir();
 public abstract File getExternalFilesDir(String kind);
 public abstract AssetManager getAssets();
}
