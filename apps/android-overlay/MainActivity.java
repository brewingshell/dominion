package net.dominion.client;

import android.net.http.SslError;
import android.os.Bundle;
import android.webkit.SslErrorHandler;
import android.webkit.WebView;
import android.webkit.WebViewClient;

import com.getcapacitor.BridgeActivity;

/**
 * Capacitor host activity for dominion.
 *
 * <p>The portal is served over HTTPS with a self-signed local CA. When the CA
 * is bundled and referenced from network_security_config.xml, the WebView
 * verifies the chain normally and the SSL override below never runs. As a
 * fallback the override tolerates the certificate solely for {@link #ALLOWED_HOST}
 * (encryption without certificate validation).
 */
public class MainActivity extends BridgeActivity {

    /** Set to the portal host; SSL errors are only tolerated for this host. */
    private static final String ALLOWED_HOST = "YOUR-HOST";

    @Override
    public void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        WebView webView = getBridge().getWebView();
        final WebViewClient base = webView.getWebViewClient();
        webView.setWebViewClient(new SslTolerantClient(base));
    }

    /** Delegates everything to the Capacitor client but tolerates our TLS error. */
    private static final class SslTolerantClient extends WebViewClient {
        private final WebViewClient delegate;

        SslTolerantClient(WebViewClient delegate) {
            this.delegate = delegate;
        }

        @Override
        public void onReceivedSslError(WebView view, SslErrorHandler handler, SslError error) {
            if (ALLOWED_HOST.equals(hostOf(error))) {
                handler.proceed();
                return;
            }
            if (delegate != null) {
                delegate.onReceivedSslError(view, handler, error);
            } else {
                super.onReceivedSslError(view, handler, error);
            }
        }
    }

    private static String hostOf(SslError error) {
        try {
            return new java.net.URI(error.getUrl()).getHost();
        } catch (Exception e) {
            return "";
        }
    }
}
