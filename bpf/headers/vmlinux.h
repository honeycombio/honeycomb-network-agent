
#if defined(__TARGET_ARCH_x86)
#include "vmlinux-x86_64.h"
#elif defined(__TARGET_ARCH_arm64)
#include "vmlinux-arm64.h"
#elif defined(__x86_64__)
#include "vmlinux-x86_64.h"
#elif defined(__aarch64__)
#include "vmlinux-arm64.h"
#else
error "Unsupported architecture"
#endif
