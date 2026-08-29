package dev.maestro.animationtest;

import android.app.Activity;
import android.os.Bundle;
import android.webkit.WebView;
import android.webkit.WebViewClient;

public class MainActivity extends Activity {

    private static final String SPINNER_HTML =
            "<!doctype html>\n" +
            "<html><head><meta charset=\"utf-8\">\n" +
            "<style>\n" +
            "  html,body{margin:0;height:100%;background:#fff;display:flex;align-items:center;justify-content:center}\n" +
            "  .spinner{width:120px;height:120px;border:16px solid #eee;border-top-color:#3498db;border-radius:50%;animation:spin 1s linear infinite}\n" +
            "  @keyframes spin{to{transform:rotate(360deg)}}\n" +
            "</style></head><body><div class=\"spinner\"></div></body></html>";

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        setContentView(R.layout.activity_main);

        WebView webView = findViewById(R.id.webview);
        webView.setWebViewClient(new WebViewClient());
        webView.getSettings().setJavaScriptEnabled(true);
        webView.getSettings().setDomStorageEnabled(true);
        webView.loadDataWithBaseURL(null, SPINNER_HTML, "text/html", "utf-8", null);
    }
}
