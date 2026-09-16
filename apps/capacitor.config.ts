import type { CapacitorConfig } from "@capacitor/cli";

const config: CapacitorConfig = {
  appId: "net.dominion.client",
  appName: "dominion",
  webDir: "www",
  android: {
    // The portal may be reached over plain HTTP (the app's own origin is
    // https://localhost), which is mixed content; allow it.
    allowMixedContent: true,
    // Marks the shell so the server page can show the "Change server" button.
    appendUserAgent: "dominion-shell/1.0",
  },
  server: {
    androidScheme: "https",
  },
};

export default config;
