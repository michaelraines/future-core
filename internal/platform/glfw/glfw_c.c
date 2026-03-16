// Compiles vendored GLFW 3.4 source for X11 on Linux/BSD.
// _GLFW_X11 is set via CGo CFLAGS in glfwapi_cgo.go.
// This file is only compiled when CGo is active (linux || freebsd builds).

#include "cglfw/context.c"
#include "cglfw/init.c"
#include "cglfw/input.c"
#include "cglfw/monitor.c"
#include "cglfw/window.c"
#include "cglfw/vulkan.c"
#include "cglfw/platform.c"

// X11 platform
#include "cglfw/x11_init.c"
#include "cglfw/x11_monitor.c"
#include "cglfw/x11_window.c"
#include "cglfw/xkb_unicode.c"

// GL contexts
#include "cglfw/glx_context.c"
#include "cglfw/egl_context.c"
#include "cglfw/osmesa_context.c"

// POSIX support
#include "cglfw/posix_thread.c"
#include "cglfw/posix_time.c"
#include "cglfw/posix_module.c"
#include "cglfw/posix_poll.c"

// Linux joystick
#include "cglfw/linux_joystick.c"

// Null platform is compiled in glfw_null.c (separate translation unit)
// to avoid static function name collisions with x11_window.c.
