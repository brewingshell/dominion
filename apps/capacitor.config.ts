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
    // The version is the client capability version: the portal uses it to gate
    // features a given build can service (see canManageServers in web/app.js).
    appendUserAgent: "dominion-shell/1.1",
  },
  server: {
    androidScheme: "https",
  },
};

export default config;
