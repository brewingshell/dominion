// Electron main process for the dominion desktop shell.
//
// The portal serves HTTPS with a self-signed local CA, so Chromium will reject
// the certificate. We accept it (equivalent to the Android shell's
// onReceivedSslError handler). There is no fingerprint UI by design; installing
// the CA into the OS trust store upgrades this to full verification.
const { app, BrowserWindow, shell } = require("electron");
const path = require("node:path");

app.on("certificate-error", (event, _webContents, url, _error, _cert, callback) => {
  event.preventDefault();
  callback(true);
});

function createWindow() {
  const win = new BrowserWindow({
    width: 1100,
    height: 720,
    backgroundColor: "#0b0e14",
    title: "dominion",
  });

  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url);
    return { action: "deny" };
  });

  win.loadFile(path.join(__dirname, "..", "www", "index.html"));
}

app.whenReady().then(() => {
  createWindow();
  app.on("activate", () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow();
  });
});

app.on("window-all-closed", () => app.quit());
