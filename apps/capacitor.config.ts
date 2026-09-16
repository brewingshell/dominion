import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "net.dominion.client",
  appName: "dominion",
  webDir: "www",
  android: {
    // The portal is served over HTTPS with a local CA; the app trusts it via
    // the network security config, so cleartext is not needed.
    allowMixedContent: false,
  },
  server: {
    androidScheme: "https",
  },
};

export default config;
