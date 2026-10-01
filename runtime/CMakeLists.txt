cmake_minimum_required(VERSION 3.10)
project(goprotect_runtime C)

set(CMAKE_C_STANDARD 11)

# Android NDK 配置
if(ANDROID)
    set(CMAKE_C_FLAGS "${CMAKE_C_FLAGS} -fPIC -fvisibility=hidden")
endif()

# 库目标
add_library(goprotect_runtime STATIC
    src/integrity.c
    src/antidebug.c
    src/vm_entry.c
    src/hooks.c
)

target_include_directories(goprotect_runtime PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}/include
)

# Android 特定链接
if(ANDROID)
    target_link_libraries(goprotect_runtime
        log
    )
endif()
