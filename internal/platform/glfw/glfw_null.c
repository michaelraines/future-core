// Compiles vendored GLFW 3.4 null-platform source as a separate translation
// unit.  The null_window.c file defines static functions (acquireMonitor,
// releaseMonitor, createNativeWindow) that conflict with identically-named
// statics in x11_window.c, so they cannot share a translation unit.

#include "cglfw/null_init.c"
#include "cglfw/null_monitor.c"
#include "cglfw/null_window.c"
#include "cglfw/null_joystick.c"
