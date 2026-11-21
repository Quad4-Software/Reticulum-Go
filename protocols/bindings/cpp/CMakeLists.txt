# SPDX-License-Identifier: LicenseRef-Reticulum

cmake_minimum_required(VERSION 3.16)
project(rrc_cpp LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

get_filename_component(RRC_ROOT "${CMAKE_CURRENT_SOURCE_DIR}/../.." ABSOLUTE)
set(RRC_INCLUDE_DIR "${RRC_ROOT}/include")
set(RRC_LIB_DIR "${RRC_ROOT}/bin")

find_library(RRC_LIBRARY NAMES rrc PATHS "${RRC_LIB_DIR}" NO_DEFAULT_PATH REQUIRED)
find_library(MF_LIBRARY NAMES mf PATHS "${RRC_LIB_DIR}" NO_DEFAULT_PATH REQUIRED)
find_library(LXMF_LIBRARY NAMES lxmf PATHS "${RRC_LIB_DIR}" NO_DEFAULT_PATH REQUIRED)
find_library(LXST_LIBRARY NAMES lxst PATHS "${RRC_LIB_DIR}" NO_DEFAULT_PATH REQUIRED)

add_executable(rrc_cpp_smoke tests/smoke_test.cpp)
target_include_directories(rrc_cpp_smoke PRIVATE "${RRC_INCLUDE_DIR}")
target_link_libraries(rrc_cpp_smoke PRIVATE ${RRC_LIBRARY})
set_target_properties(rrc_cpp_smoke PROPERTIES BUILD_RPATH "${RRC_LIB_DIR}")

add_executable(rrc_cpp_mf_smoke tests/mf_smoke_test.cpp)
target_include_directories(rrc_cpp_mf_smoke PRIVATE "${RRC_INCLUDE_DIR}")
target_link_libraries(rrc_cpp_mf_smoke PRIVATE ${MF_LIBRARY})
set_target_properties(rrc_cpp_mf_smoke PROPERTIES BUILD_RPATH "${RRC_LIB_DIR}")

add_executable(rrc_cpp_lxmf_smoke tests/lxmf_smoke_test.cpp)
target_include_directories(rrc_cpp_lxmf_smoke PRIVATE "${RRC_INCLUDE_DIR}")
target_link_libraries(rrc_cpp_lxmf_smoke PRIVATE ${LXMF_LIBRARY})
set_target_properties(rrc_cpp_lxmf_smoke PROPERTIES BUILD_RPATH "${RRC_LIB_DIR}")

add_executable(rrc_cpp_lxmf_interop tests/lxmf_interop_test.cpp)
target_include_directories(rrc_cpp_lxmf_interop PRIVATE "${RRC_INCLUDE_DIR}")
target_link_libraries(rrc_cpp_lxmf_interop PRIVATE ${LXMF_LIBRARY})
set_target_properties(rrc_cpp_lxmf_interop PROPERTIES BUILD_RPATH "${RRC_LIB_DIR}")

add_executable(rrc_cpp_lxst_smoke tests/lxst_smoke_test.cpp)
target_include_directories(rrc_cpp_lxst_smoke PRIVATE "${RRC_INCLUDE_DIR}")
target_link_libraries(rrc_cpp_lxst_smoke PRIVATE ${LXST_LIBRARY})
set_target_properties(rrc_cpp_lxst_smoke PROPERTIES BUILD_RPATH "${RRC_LIB_DIR}")

enable_testing()
add_test(NAME rrc_cpp_smoke COMMAND rrc_cpp_smoke)
add_test(NAME rrc_cpp_mf_smoke COMMAND rrc_cpp_mf_smoke)
add_test(NAME rrc_cpp_lxmf_smoke COMMAND rrc_cpp_lxmf_smoke)
add_test(NAME rrc_cpp_lxmf_interop COMMAND rrc_cpp_lxmf_interop
	WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}")
add_test(NAME rrc_cpp_lxst_smoke COMMAND rrc_cpp_lxst_smoke)
