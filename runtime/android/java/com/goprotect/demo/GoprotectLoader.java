package com.goprotect.demo;

import android.app.Application;
import android.content.Context;
import android.util.Log;
import dalvik.system.InMemoryDexClassLoader;
import java.nio.ByteBuffer;

/**
 * Goprotect demo 加载器（ADR-0001 的 Android 侧示例）。
 *
 * 用法：
 *   1. protector 以默认配置（embed_key=false，compress_before_encrypt=false）
 *      加固 App；密钥落 <output>.key，经发布流程注入 native 库的拆分常量；
 *   2. AndroidManifest.xml 的 application 节点指向本类（或在自己的
 *      Application 里复用 loadProtectedDex）；
 *   3. 首个 Activity/入口类从返回的 ClassLoader 反射加载。
 *
 * 这是演示代码：密钥分片只是"提高静态提取成本"的混淆，生产环境应替换为
 * 服务端下发 / 签名派生 / 硬件密钥等注入通道。
 */
public class GoprotectLoader extends Application {
    private static final String TAG = "GoprotectLoader";

    static {
        System.loadLibrary("goprotect_loader");
    }

    private static native byte[] nativeLoadDex(Context ctx);

    private InMemoryDexClassLoader dexLoader;

    public static InMemoryDexClassLoader loadProtectedDex(Context ctx) {
        byte[] dex = nativeLoadDex(ctx);
        if (dex == null) {
            Log.e(TAG, "protected dex decrypt failed (key mismatch or tampered payload)");
            return null;
        }
        ByteBuffer buf = ByteBuffer.allocateDirect(dex.length);
        buf.put(dex);
        buf.flip();
        return new InMemoryDexClassLoader(new ByteBuffer[] { buf },
                GoprotectLoader.class.getClassLoader());
    }

    @Override
    public void onCreate() {
        super.onCreate();
        dexLoader = loadProtectedDex(this);
        if (dexLoader != null) {
            Log.i(TAG, "protected dex loaded in memory");
            // 示例：Class<?> entry = dexLoader.loadClass("com.example.app.Main");
        }
    }

    public InMemoryDexClassLoader getDexLoader() {
        return dexLoader;
    }
}
