package net.dominion.client;

import com.getcapacitor.BridgeActivity;

/**
 * Capacitor host activity for dominion.
 *
 * <p>Deliberately does nothing beyond {@link BridgeActivity}: Capacitor's own
 * {@code BridgeWebViewClient} serves the app bundle from {@code https://localhost}
 * and handles that local certificate. Wrapping or replacing the client breaks
 * {@code shouldInterceptRequest} and the app fails to load with
 * ERR_CONNECTION_REFUSED.
 *
 * <p>Trust for the portal's self-signed CA is configured in
 * {@code res/xml/network_security_config.xml}, which trusts the bundled CA for
 * any host, since the address is entered by the user at runtime.
 */
public class MainActivity extends BridgeActivity {
}
