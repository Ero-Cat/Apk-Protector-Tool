# goprotect demo 加载器（Android/NDK）。
# 用法（应用模块的 CMakeLists.txt include 本文件或直接 add_subdirectory）：
#   set(GOPROTECT_KEY_PART0_HEX "11,22,...")  # 发布流程生成后注入
#   add_subdirectory(runtime/android)
cmake_minimum_required(VERSION 3.22)
project(goprotect_loader)

add_library(goprotect_loader SHARED
    goprotect_loader.c
    dex_decrypt.c
    crypto/aes_gcm.c
)

target_include_directories(goprotect_loader PRIVATE ${CMAKE_CURRENT_SOURCE_DIR})
target_link_libraries(goprotect_loader PRIVATE android log)
