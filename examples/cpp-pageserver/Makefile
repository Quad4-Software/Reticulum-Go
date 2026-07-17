# SPDX-License-Identifier: Apache-2.0
# Copyright (c) 2024-2026 Quad4.io

CXX ?= g++
ROOT := ../..
LIB := $(ROOT)/bin/librns.so
CPP_BIND := $(ROOT)/bindings/cpp
INC := -I$(CPP_BIND)/include -I$(ROOT)/include
CXXFLAGS ?= -std=c++17 -Wall -Wextra -O2
CONFIG ?= config.example

.PHONY: all clean librns run

all: cpp-pageserver

librns: $(LIB)

$(LIB):
	cd $(ROOT) && task build-librns

cpp-pageserver: main.cpp $(LIB) $(CPP_BIND)/src/event.cpp
	$(CXX) $(CXXFLAGS) $(INC) -L$(ROOT)/bin -Wl,-rpath,'$$ORIGIN/../../bin' \
		-o $@ main.cpp $(CPP_BIND)/src/event.cpp -lrns

run: cpp-pageserver
	./cpp-pageserver -c $(CONFIG) $(ARGS)

clean:
	rm -f cpp-pageserver
