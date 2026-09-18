//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>
#include <stdlib.h>

// dominion_set_app_identity names the application before any window exists.
// GLib's program name becomes the Wayland app_id and the X11 WM_CLASS instance;
// gdk_set_program_class sets the WM_CLASS class. A compositor uses the app_id
// to associate the window with the installed .desktop entry and resolve its
// icon.
static void dominion_set_app_identity(void) {
	g_set_prgname("dominion");
	g_set_application_name("dominion");
	gdk_set_program_class("dominion");
}

// dominion_set_window_icon loads the icon from disk and sets it on the window.
// Several sizes are supplied because GTK only publishes icons up to a maximum
// size in _NET_WM_ICON and silently drops larger pixbufs; starting small
// guarantees the title bar gets an icon on X11. On Wayland the protocol has no
// per-window icon, so the compositor ignores this and matches the app_id
// against an installed .desktop file instead.
static void dominion_set_window_icon(void *win, const char *path) {
	if (win == NULL || path == NULL) {
		return;
	}
	static const int sizes[] = {16, 24, 32, 48, 64, 128, 256};
	GList *list = NULL;
	GError *err = NULL;
	for (size_t i = 0; i < G_N_ELEMENTS(sizes); i++) {
		GdkPixbuf *pb = gdk_pixbuf_new_from_file_at_scale(path, sizes[i], sizes[i], TRUE, &err);
		if (pb != NULL) {
			list = g_list_append(list, pb);
		} else if (err != NULL) {
			g_warning("could not load icon %s at %dpx: %s", path, sizes[i], err->message);
			g_error_free(err);
			err = NULL;
		}
	}
	if (list != NULL) {
		gtk_window_set_icon_list(GTK_WINDOW(win), list);
		// GTK keeps the pixbufs; only the list cells are ours to free.
		g_list_free(list);
	}
}
*/
import "C"

import (
	"os"
	"path/filepath"
	"unsafe"
)

// setAppIdentity tells GLib/GTK who the application is. It must run before the
// GtkWindow is created (webview.New), because WM_CLASS and the Wayland app_id
// are fixed when the window is realized.
func setAppIdentity() {
	C.dominion_set_app_identity()
}

// setWindowIcon sets the window icon from a PNG on disk. On X11 this is the
// _NET_WM_ICON property the window manager draws in the title bar; on Wayland
// it is a no-op in practice and the icon comes from the app_id (see
// apps/README.md). Best-effort: a missing icon leaves the default in place.
func setWindowIcon(win unsafe.Pointer) {
	path := appIconPath()
	if path == "" {
		return
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	C.dominion_set_window_icon(win, cpath)
}

// appIconPath locates the packaged icon. Inside an AppImage it is at the
// AppDir root (and under hicolor); a build-tree run falls back to a sibling
// dominion.png. Returns "" when none is found, so the window keeps the
// toolkit default.
func appIconPath() string {
	var candidates []string
	if appDir := os.Getenv("APPDIR"); appDir != "" {
		candidates = append(candidates,
			filepath.Join(appDir, "dominion.png"),
			filepath.Join(appDir, "usr", "share", "icons", "hicolor", "256x256", "apps", "dominion.png"),
		)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "dominion.png"),
			filepath.Join(dir, "..", "dominion.png"),
		)
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}
