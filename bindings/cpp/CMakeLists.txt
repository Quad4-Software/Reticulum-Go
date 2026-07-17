# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

cmake_minimum_required(VERSION 3.16)
project(rns_cpp LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

option(RNS_CPP_BUILD_TESTS "Build C++ binding tests" ON)

get_filename_component(RNS_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../.." ABSOLUTE)
set(RNS_INCLUDE_DIR "${RNS_ROOT}/include" CACHE PATH "Path to rns.h")
set(RNS_LIB_DIR "${RNS_ROOT}/bin" CACHE PATH "Path to librns.so")

find_library(RNS_LIBRARY NAMES rns PATHS "${RNS_LIB_DIR}" NO_DEFAULT_PATH REQUIRED)
if(NOT EXISTS "${RNS_INCLUDE_DIR}/rns.h")
	message(FATAL_ERROR "rns.h not found at ${RNS_INCLUDE_DIR}/rns.h (build librns first)")
endif()

add_library(rns_cpp STATIC
	src/event.cpp
)
add_library(rns::cpp ALIAS rns_cpp)

target_include_directories(rns_cpp
	PUBLIC
		$<BUILD_INTERFACE:${CMAKE_CURRENT_SOURCE_DIR}/include>
		$<BUILD_INTERFACE:${RNS_INCLUDE_DIR}>
		$<INSTALL_INTERFACE:include>
)
target_link_libraries(rns_cpp PUBLIC ${RNS_LIBRARY})
target_compile_features(rns_cpp PUBLIC cxx_std_17)

# Ensure tests and examples find librns.so at runtime.
set(RNS_RPATH "${RNS_LIB_DIR}")

if(RNS_CPP_BUILD_TESTS)
	include(FetchContent)
	FetchContent_Declare(
		Catch2
		GIT_REPOSITORY https://github.com/catchorg/Catch2.git
		GIT_TAG v3.5.4
	)
	FetchContent_MakeAvailable(Catch2)

	enable_testing()
	add_executable(rns_cpp_tests
		tests/smoke_test.cpp
		tests/identity_test.cpp
		tests/destination_test.cpp
		tests/link_udp_test.cpp
	)
	target_include_directories(rns_cpp_tests PRIVATE
		${CMAKE_CURRENT_SOURCE_DIR}/tests
		${CMAKE_CURRENT_SOURCE_DIR}/include
		${RNS_INCLUDE_DIR}
	)
	target_link_libraries(rns_cpp_tests PRIVATE rns_cpp Catch2::Catch2WithMain)
	set_target_properties(rns_cpp_tests PROPERTIES
		BUILD_RPATH "${RNS_RPATH}"
		INSTALL_RPATH "${RNS_RPATH}"
	)
	include(CTest)
	list(APPEND CMAKE_MODULE_PATH ${catch2_SOURCE_DIR}/extras)
	include(Catch)
	catch_discover_tests(rns_cpp_tests)
endif()

include(CMakePackageConfigHelpers)
configure_file(
	${CMAKE_CURRENT_SOURCE_DIR}/cmake/RnsConfig.cmake.in
	${CMAKE_CURRENT_BINARY_DIR}/RnsConfig.cmake
	@ONLY
)
