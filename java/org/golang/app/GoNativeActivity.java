package org.golang.app;

import android.content.Intent;
import android.os.Bundle;

public class GoNativeActivity extends org.golang.app.GoNativeActivityBase {

    static {
        System.loadLibrary("croc");
    }

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        if (savedInstanceState == null) {
            handleIntent(getIntent());
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        handleIntent(intent);
    }

    private void handleIntent(Intent intent) {
        if (intent == null) {
            return;
        }
        if ((intent.getFlags() & Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY) != 0) {
            return; 
        }

        String action = intent.getAction();
        if (Intent.ACTION_VIEW.equals(action) || 
            Intent.ACTION_SEND.equals(action) ||
            Intent.ACTION_SEND_MULTIPLE.equals(action)) {
            
            processIntent(intent);
            
            if (Intent.ACTION_SEND.equals(action) || 
                Intent.ACTION_SEND_MULTIPLE.equals(action)) {
                setResult(android.app.Activity.RESULT_OK);
            }
        }
    }

    private native void processIntent(Intent intent);
}