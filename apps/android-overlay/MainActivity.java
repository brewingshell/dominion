package net.dominion.client;

import android.graphics.Bitmap;
import android.net.Uri;
import android.os.Bundle;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebResourceResponse;
import android.webkit.WebView;

import com.getcapacitor.BridgeActivity;
import com.getcapacitor.BridgeWebViewClient;

/**
 * Capacitor host activity for dominion.
 *
 * <p>Subclasses {@link BridgeWebViewClient} rather than replacing it: Capacitor
 * serves the app bundle from {@code https://localhost} through
 * {@code shouldInterceptRequest}, and dropping that hook makes the app fail
 * with ERR_CONNECTION_REFUSED.
 *
 * <p>Two adjustments:
 * <ul>
 *   <li>Remote {@code http}/{@code https} navigations load inside the WebView
 *       instead of being handed to the system browser
 *       ({@code launchIntent} would otherwise open Chrome).
 *   <li>A failed main-frame load returns to the address prompt with an error,
 *       so the user can correct the address or pick HTTP/HTTPS.
 * </ul>
 *
 * <p>Trust for the portal's self-signed CA comes from
 * {@code res/xml/network_security_config.xml}.
 */
public class MainActivity extends BridgeActivity {

    /** The app's own origin; the prompt lives here. */
    private static final String APP_ORIGIN = "https://localhost";

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);
        getBridge().getWebView().setWebViewClient(new DominionWebViewClient(getBridge()));
    }

    private static final class DominionWebViewClient extends BridgeWebViewClient {

        DominionWebViewClient(com.getcapacitor.Bridge bridge) {
            super(bridge);
        }

        /** Keep the portal loaded in-app rather than opening the system browser. */
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
            Uri url = request.getUrl();
            String scheme = url == null ? null : url.getScheme();
            if ("http".equals(scheme) || "https".equals(scheme)) {
                return false;
            }
            return super.shouldOverrideUrlLoading(view, request);
        }

        @SuppressWarnings("deprecation")
        @Override
        public boolean shouldOverrideUrlLoading(WebView view, String url) {
            if (url != null && (url.startsWith("http://") || url.startsWith("https://"))) {
                return false;
            }
            return super.shouldOverrideUrlLoading(view, url);
        }

        @Override
        public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
            if (request != null && request.isForMainFrame()) {
                view.loadUrl(APP_ORIGIN + "/?change=1&error=unreachable");
                return;
            }
            super.onReceivedError(view, request, error);
        }

        @Override
        public void onReceivedHttpError(WebView view, WebResourceRequest request, WebResourceResponse response) {
            if (request != null && request.isForMainFrame()) {
                view.loadUrl(APP_ORIGIN + "/?change=1&error=unreachable");
                return;
            }
            super.onReceivedHttpError(view, request, response);
        }

        // Bitmap import keeps the deprecated overload signature aligned.
        @Override
        public void onPageStarted(WebView view, String url, Bitmap favicon) {
            super.onPageStarted(view, url, favicon);
        }
    }
}
