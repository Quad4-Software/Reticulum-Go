# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

CXX ?= g++
ROOT := ../..
LIB := $(ROOT)/bin/librns.so
CPP_BIND := $(ROOT)/bindings/cpp
INC := -I$(CPP_BIND)/include -I$(ROOT)/include
CXXFLAGS ?= -std=c++17 -Wall -Wextra -O2

.PHONY: all clean librns

all: cpp-smoke

librns: $(LIB)

$(LIB):
	cd $(ROOT) && task build-librns

cpp-smoke: main.cpp $(LIB) $(CPP_BIND)/src/event.cpp
	$(CXX) $(CXXFLAGS) $(INC) -L$(ROOT)/bin -Wl,-rpath,'$$ORIGIN/../../bin' \
		-o $@ main.cpp $(CPP_BIND)/src/event.cpp -lrns

clean:
	rm -f cpp-smoke
